package alert

import "expvar"

var (
	engineEvaluateTotal = expvar.NewInt("alert_engine_evaluate_total")
	engineMatchedTotal  = expvar.NewInt("alert_engine_matched_total")
	engineEnqueuedTotal = expvar.NewInt("alert_engine_enqueued_total")

	workerClaimedTotal  = expvar.NewInt("alert_worker_claimed_total")
	workerRequeuedTotal = expvar.NewInt("alert_worker_requeued_processing_total")

	workerSentTotalByChannel   = expvar.NewMap("alert_worker_sent_total_by_channel")
	workerFailedTotalByChannel = expvar.NewMap("alert_worker_failed_total_by_channel")
	workerRetryTotalByChannel  = expvar.NewMap("alert_worker_retry_total_by_channel")

	alertCleanupDeletedDeliveriesTotal = expvar.NewInt("alert_cleanup_deleted_deliveries_total")
	alertCleanupDeletedStatesTotal     = expvar.NewInt("alert_cleanup_deleted_states_total")
)

func addMapCounter(m *expvar.Map, key string, delta int64) {
	if m == nil || key == "" || delta == 0 {
		return
	}
	v := m.Get(key)
	if v == nil {
		i := new(expvar.Int)
		i.Set(delta)
		m.Set(key, i)
		return
	}
	if i, ok := v.(*expvar.Int); ok {
		i.Add(delta)
	}
}
