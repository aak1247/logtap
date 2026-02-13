package alert

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/smtp"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Worker struct {
	DB         *gorm.DB
	HTTPClient *http.Client
	Now        func() time.Time
	Config     config.Config
}

func NewWorker(db *gorm.DB, cfg config.Config) *Worker {
	return &Worker{
		DB: db,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Prevent redirect-based SSRF bypass.
				return http.ErrUseLastResponse
			},
		},
		Now:    time.Now,
		Config: cfg,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.DB == nil {
		return nil
	}
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()

	nextCleanup := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if w.shouldRunCleanup(nextCleanup) {
				nextCleanup = w.Now().UTC().Add(w.cleanupInterval())
				cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				_ = w.cleanupOnce(cleanupCtx)
				cancel()
			}
			_, _ = w.ProcessOnce(ctx, 50)
		}
	}
}

func (w *Worker) cleanupInterval() time.Duration {
	if w == nil {
		return time.Hour
	}
	if w.Config.AlertCleanupInterval > 0 {
		return w.Config.AlertCleanupInterval
	}
	return time.Hour
}

func (w *Worker) shouldRunCleanup(next time.Time) bool {
	if w == nil {
		return false
	}
	if w.Config.AlertDeliveriesRetentionDays <= 0 && w.Config.AlertStatesRetentionDays <= 0 {
		return false
	}
	now := w.Now().UTC()
	return next.IsZero() || !now.Before(next)
}

func (w *Worker) ProcessOnce(ctx context.Context, limit int) (int, error) {
	if w == nil || w.DB == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 50
	}
	now := w.Now().UTC()

	// Requeue stuck processing items (e.g. worker crash) so they can be retried.
	const processingLease = 2 * time.Minute
	requeue := w.DB.WithContext(ctx).
		Model(&model.AlertDelivery{}).
		Where("status = ? AND updated_at < ?", "processing", now.Add(-processingLease)).
		Updates(map[string]any{
			"status":          "pending",
			"next_attempt_at": now,
			"updated_at":      now,
		})
	if requeue.Error == nil && requeue.RowsAffected > 0 {
		workerRequeuedTotal.Add(requeue.RowsAffected)
	}

	items, err := w.claimPending(ctx, limit, now)
	if err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}
	workerClaimedTotal.Add(int64(len(items)))

	processed := 0
	for _, d := range items {
		processed++

		err := w.send(ctx, d)
		if err == nil {
			_ = w.DB.WithContext(ctx).Model(&model.AlertDelivery{}).Where("id = ?", d.ID).
				Updates(map[string]any{"status": "sent", "updated_at": now, "last_error": ""}).Error
			addMapCounter(workerSentTotalByChannel, d.ChannelType, 1)
			continue
		}

		attempts := d.Attempts + 1
		status := "pending"
		next := now.Add(backoffDelay(attempts))
		if isPermanent(err) {
			status = "failed"
			next = now
		} else if attempts >= 10 {
			status = "failed"
			next = now
		}
		_ = w.DB.WithContext(ctx).Model(&model.AlertDelivery{}).Where("id = ?", d.ID).
			Updates(map[string]any{
				"attempts":        attempts,
				"next_attempt_at": next,
				"status":          status,
				"last_error":      err.Error(),
				"updated_at":      now,
			}).Error
		if status == "failed" {
			addMapCounter(workerFailedTotalByChannel, d.ChannelType, 1)
		} else {
			addMapCounter(workerRetryTotalByChannel, d.ChannelType, 1)
		}
	}
	return processed, nil
}

func (w *Worker) cleanupOnce(ctx context.Context) error {
	if w == nil || w.DB == nil {
		return nil
	}

	now := w.Now().UTC()
	deliveryDays := w.Config.AlertDeliveriesRetentionDays
	stateDays := w.Config.AlertStatesRetentionDays
	if deliveryDays <= 0 && stateDays <= 0 {
		return nil
	}

	isPostgres := strings.EqualFold(strings.TrimSpace(w.DB.Dialector.Name()), "postgres")
	const advisoryLockID int64 = 730731 // stable, app-scoped

	run := func(db *gorm.DB) error {
		if deliveryDays > 0 {
			before := now.Add(-time.Duration(deliveryDays) * 24 * time.Hour)
			n, err := deleteOldDeliveriesBatched(ctx, db, before, 5000, 20)
			if err != nil {
				return err
			}
			if n > 0 {
				alertCleanupDeletedDeliveriesTotal.Add(n)
			}
		}
		if stateDays > 0 {
			before := now.Add(-time.Duration(stateDays) * 24 * time.Hour)
			n, err := deleteOldStatesBatched(ctx, db, before, 5000, 20)
			if err != nil {
				return err
			}
			if n > 0 {
				alertCleanupDeletedStatesTotal.Add(n)
			}
		}
		return nil
	}

	if !isPostgres {
		return run(w.DB.WithContext(ctx))
	}

	return w.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ok bool
		if err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", advisoryLockID).Scan(&ok).Error; err != nil {
			return err
		}
		if !ok {
			return nil
		}
		return run(tx)
	})
}

func deleteOldDeliveriesBatched(ctx context.Context, db *gorm.DB, before time.Time, batchSize int, maxBatches int) (int64, error) {
	if db == nil {
		return 0, nil
	}
	if batchSize <= 0 {
		batchSize = 5000
	}
	if maxBatches <= 0 {
		maxBatches = 1
	}
	var total int64
	for i := 0; i < maxBatches; i++ {
		res := db.WithContext(ctx).Exec(`
			DELETE FROM alert_deliveries
			WHERE id IN (
				SELECT id FROM alert_deliveries
				WHERE created_at < ? AND status IN ('sent','failed')
				ORDER BY id
				LIMIT ?
			)
		`, before, batchSize)
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
		if res.RowsAffected == 0 {
			break
		}
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
	}
	return total, nil
}

func deleteOldStatesBatched(ctx context.Context, db *gorm.DB, before time.Time, batchSize int, maxBatches int) (int64, error) {
	if db == nil {
		return 0, nil
	}
	if batchSize <= 0 {
		batchSize = 5000
	}
	if maxBatches <= 0 {
		maxBatches = 1
	}
	var total int64
	for i := 0; i < maxBatches; i++ {
		res := db.WithContext(ctx).Exec(`
			DELETE FROM alert_states
			WHERE id IN (
				SELECT id FROM alert_states
				WHERE last_seen_at < ?
				ORDER BY id
				LIMIT ?
			)
		`, before, batchSize)
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
		if res.RowsAffected == 0 {
			break
		}
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
	}
	return total, nil
}

func (w *Worker) claimPending(ctx context.Context, limit int, now time.Time) ([]model.AlertDelivery, error) {
	var items []model.AlertDelivery
	if w == nil || w.DB == nil {
		return nil, nil
	}

	// Prefer row locking with SKIP LOCKED on Postgres to safely support multiple workers.
	isPostgres := strings.EqualFold(strings.TrimSpace(w.DB.Dialector.Name()), "postgres")

	err := w.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.WithContext(ctx).
			Where("status = ? AND next_attempt_at <= ?", "pending", now).
			Order("id ASC").
			Limit(limit)

		if isPostgres {
			q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := q.Find(&items).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		ids := make([]int, 0, len(items))
		for _, it := range items {
			ids = append(ids, it.ID)
		}
		if err := tx.WithContext(ctx).
			Model(&model.AlertDelivery{}).
			Where("id IN ? AND status = ?", ids, "pending").
			Updates(map[string]any{"status": "processing", "updated_at": now}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Status = "processing"
	}
	return items, nil
}

func backoffDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	d := 2 * time.Second
	for i := 1; i < attempt; i++ {
		d *= 2
		if d > 30*time.Minute {
			return 30 * time.Minute
		}
	}
	return d
}

func (w *Worker) send(ctx context.Context, d model.AlertDelivery) error {
	switch d.ChannelType {
	case "wecom":
		return w.sendWecom(ctx, d.Target, d.Title, d.Content)
	case "webhook":
		return w.sendWebhook(ctx, d.ID, d.Target, d.ProjectID, d.RuleID, d.Title, d.Content)
	case "sms":
		return w.sendSMS(ctx, d.Target, d.Title, d.Content)
	case "email":
		return w.sendEmail(d.Target, d.Title, d.Content)
	default:
		return fmt.Errorf("unknown channel_type=%q", d.ChannelType)
	}
}

func (w *Worker) sendWecom(ctx context.Context, webhookURL string, title string, content string) error {
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return permanent(errors.New("wecom webhook_url empty"))
	}
	body, _ := json.Marshal(map[string]any{
		"msgtype": "text",
		"text": map[string]any{
			"content": title + "\n" + content,
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := w.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("wecom http %d", res.StatusCode)
	}
	return nil
}

func (w *Worker) sendWebhook(ctx context.Context, deliveryID int, urlStr string, projectID int, ruleID int, title string, content string) error {
	urlStr = strings.TrimSpace(urlStr)
	if urlStr == "" {
		return permanent(errors.New("webhook url empty"))
	}
	if err := ValidateWebhookURL(ctx, urlStr, WebhookValidationOptions{
		AllowLoopback:   w.Config.WebhookAllowLoopback,
		AllowPrivateIPs: w.Config.WebhookAllowPrivateIPs,
		AllowlistCIDRs:  w.Config.WebhookAllowlistCIDRs,
	}); err != nil {
		return permanent(err)
	}
	body, _ := json.Marshal(map[string]any{
		"deliveryId": deliveryID,
		"projectId":  projectID,
		"ruleId":     ruleID,
		"title":      title,
		"content":    content,
		"sentAt":     w.Now().UTC().Format(time.RFC3339Nano),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(body))
	if err != nil {
		return permanent(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if deliveryID > 0 {
		req.Header.Set("X-Logtap-Delivery-Id", strconv.Itoa(deliveryID))
	}
	res, err := w.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("webhook http %d", res.StatusCode)
	}
	return nil
}

func (w *Worker) sendSMS(ctx context.Context, phone string, title string, content string) error {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return permanent(errors.New("sms phone empty"))
	}

	switch strings.ToLower(strings.TrimSpace(w.Config.SMSProvider)) {
	case "aliyun":
		return w.sendAliyunSMS(ctx, phone, title, content)
	case "tencent":
		return w.sendTencentSMS(ctx, phone, title, content)
	case "":
		return permanent(errors.New("SMS_PROVIDER not configured"))
	default:
		return permanent(fmt.Errorf("unsupported SMS_PROVIDER=%q", w.Config.SMSProvider))
	}
}

func (w *Worker) sendEmail(to string, subject string, body string) error {
	host := strings.TrimSpace(w.Config.SMTPHost)
	if host == "" {
		return permanent(errors.New("SMTP_HOST not configured"))
	}
	port := w.Config.SMTPPort
	if port <= 0 {
		port = 587
	}
	from := strings.TrimSpace(w.Config.SMTPFrom)
	if from == "" {
		return permanent(errors.New("SMTP_FROM not configured"))
	}

	to = strings.TrimSpace(to)
	if to == "" {
		return permanent(errors.New("email to empty"))
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	msg := []byte("To: " + to + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" + body + "\r\n")

	var auth smtp.Auth
	if strings.TrimSpace(w.Config.SMTPUsername) != "" {
		auth = smtp.PlainAuth("", w.Config.SMTPUsername, w.Config.SMTPPassword, host)
	}
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

func permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

func isPermanent(err error) bool {
	var pe permanentError
	return errors.As(err, &pe)
}

func (w *Worker) sendAliyunSMS(ctx context.Context, phone string, title string, content string) error {
	ak := strings.TrimSpace(w.Config.AliyunSMSAccessKeyID)
	sk := strings.TrimSpace(w.Config.AliyunSMSAccessKeySecret)
	sign := strings.TrimSpace(w.Config.AliyunSMSSignName)
	tpl := strings.TrimSpace(w.Config.AliyunSMSTemplateCode)
	if ak == "" || sk == "" || sign == "" || tpl == "" {
		return permanent(errors.New("aliyun sms not configured (need ALIYUN_SMS_ACCESS_KEY_ID/SECRET, ALIYUN_SMS_SIGN_NAME, ALIYUN_SMS_TEMPLATE_CODE)"))
	}
	region := strings.TrimSpace(w.Config.AliyunSMSRegion)
	if region == "" {
		region = "cn-hangzhou"
	}

	templateParam, _ := json.Marshal(map[string]string{
		"title":   title,
		"content": content,
	})

	params := map[string]string{
		"AccessKeyId":      ak,
		"Action":           "SendSms",
		"Format":           "JSON",
		"PhoneNumbers":     phone,
		"RegionId":         region,
		"SignName":         sign,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   uuid.NewString(),
		"SignatureVersion": "1.0",
		"TemplateCode":     tpl,
		"TemplateParam":    string(templateParam),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"Version":          "2017-05-25",
	}

	query := aliyunBuildSignedQuery(params, sk, http.MethodGet)
	endpoint := "https://dysmsapi.aliyuncs.com/"
	reqURL := endpoint + "?" + query

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	res, err := w.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("aliyun sms http %d", res.StatusCode)
	}
	var resp struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	_ = json.NewDecoder(res.Body).Decode(&resp)
	if !strings.EqualFold(strings.TrimSpace(resp.Code), "OK") {
		msg := strings.TrimSpace(resp.Message)
		if msg == "" {
			msg = "unknown"
		}
		return fmt.Errorf("aliyun sms code=%s msg=%s", strings.TrimSpace(resp.Code), msg)
	}
	return nil
}

func aliyunBuildSignedQuery(params map[string]string, accessKeySecret string, method string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	canonical := make([]string, 0, len(keys))
	for _, k := range keys {
		canonical = append(canonical, aliyunPercentEncode(k)+"="+aliyunPercentEncode(params[k]))
	}
	canonicalQuery := strings.Join(canonical, "&")

	stringToSign := strings.ToUpper(method) + "&" + aliyunPercentEncode("/") + "&" + aliyunPercentEncode(canonicalQuery)
	mac := hmac.New(sha1.New, []byte(accessKeySecret+"&"))
	_, _ = mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	all := make(map[string]string, len(params)+1)
	for k, v := range params {
		all[k] = v
	}
	all["Signature"] = signature

	keys = keys[:0]
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, aliyunPercentEncode(k)+"="+aliyunPercentEncode(all[k]))
	}
	return strings.Join(out, "&")
}

func aliyunPercentEncode(s string) string {
	// Aliyun RPC percent-encoding: space => %20, ~ kept, no '+'.
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func (w *Worker) sendTencentSMS(ctx context.Context, phone string, title string, content string) error {
	secretID := strings.TrimSpace(w.Config.TencentSMSSecretID)
	secretKey := strings.TrimSpace(w.Config.TencentSMSSecretKey)
	appID := strings.TrimSpace(w.Config.TencentSMSAppID)
	sign := strings.TrimSpace(w.Config.TencentSMSSignName)
	tpl := strings.TrimSpace(w.Config.TencentSMSTemplateID)
	if secretID == "" || secretKey == "" || appID == "" || sign == "" || tpl == "" {
		return permanent(errors.New("tencent sms not configured (need TENCENT_SMS_SECRET_ID/KEY, TENCENT_SMS_APP_ID, TENCENT_SMS_SIGN_NAME, TENCENT_SMS_TEMPLATE_ID)"))
	}
	region := strings.TrimSpace(w.Config.TencentSMSRegion)
	if region == "" {
		region = "ap-guangzhou"
	}

	body, _ := json.Marshal(map[string]any{
		"PhoneNumberSet":   []string{phone},
		"SmsSdkAppId":      appID,
		"SignName":         sign,
		"TemplateId":       tpl,
		"TemplateParamSet": []string{title, content},
	})

	host := "sms.tencentcloudapi.com"
	service := "sms"
	action := "SendSms"
	version := "2021-01-11"
	timestamp := time.Now().UTC().Unix()
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")

	auth := tencentAuthorization(secretID, secretKey, host, service, timestamp, body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+host, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", version)
	req.Header.Set("X-TC-Region", region)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("Authorization", auth)

	res, err := w.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("tencent sms http %d", res.StatusCode)
	}

	var resp struct {
		Response struct {
			Error *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error,omitempty"`
			SendStatusSet []struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"SendStatusSet"`
		} `json:"Response"`
	}
	_ = json.NewDecoder(res.Body).Decode(&resp)
	if resp.Response.Error != nil {
		return fmt.Errorf("tencent sms error=%s msg=%s", strings.TrimSpace(resp.Response.Error.Code), strings.TrimSpace(resp.Response.Error.Message))
	}
	if len(resp.Response.SendStatusSet) == 0 {
		return fmt.Errorf("tencent sms empty status (date=%s)", date)
	}
	code := strings.TrimSpace(resp.Response.SendStatusSet[0].Code)
	if !strings.EqualFold(code, "Ok") {
		msg := strings.TrimSpace(resp.Response.SendStatusSet[0].Message)
		if msg == "" {
			msg = "unknown"
		}
		return fmt.Errorf("tencent sms code=%s msg=%s", code, msg)
	}
	return nil
}

func tencentAuthorization(secretID, secretKey, host, service string, timestamp int64, payload []byte) string {
	canonicalURI := "/"
	canonicalQuery := ""
	canonicalHeaders := "content-type:application/json\nhost:" + host + "\n"
	signedHeaders := "content-type;host"
	hashedPayload := sha256Hex(payload)
	canonicalRequest := strings.Join([]string{
		"POST",
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		hashedPayload,
	}, "\n")

	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	credentialScope := date + "/" + service + "/tc3_request"
	stringToSign := strings.Join([]string{
		"TC3-HMAC-SHA256",
		strconv.FormatInt(timestamp, 10),
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	secretDate := hmacSHA256([]byte("TC3"+secretKey), []byte(date))
	secretService := hmacSHA256(secretDate, []byte(service))
	secretSigning := hmacSHA256(secretService, []byte("tc3_request"))
	signature := hexEncode(hmacSHA256(secretSigning, []byte(stringToSign)))

	return fmt.Sprintf(
		"TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		secretID,
		credentialScope,
		signedHeaders,
		signature,
	)
}

func hmacSHA256(key []byte, msg []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(msg)
	return mac.Sum(nil)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hexEncode(sum[:])
}

func hexEncode(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hex[v>>4]
		out[i*2+1] = hex[v&0x0f]
	}
	return string(out)
}
