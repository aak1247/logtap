package query

import (
	"errors"
	"net/http"

	"github.com/aak1247/logtap/internal/detector"
	"github.com/gin-gonic/gin"
)

func ListDetectorsHandler(svc *detector.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			respondErr(c, http.StatusServiceUnavailable, "detector service not configured")
			return
		}
		items, err := svc.ListDescriptors()
		if err != nil {
			if errors.Is(err, detector.ErrServiceNotConfigured) {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

func GetDetectorSchemaHandler(svc *detector.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			respondErr(c, http.StatusServiceUnavailable, "detector service not configured")
			return
		}
		schema, err := svc.GetSchema(c.Param("detectorType"))
		if err != nil {
			switch {
			case errors.Is(err, detector.ErrServiceNotConfigured):
				respondErr(c, http.StatusServiceUnavailable, err.Error())
			case errors.Is(err, detector.ErrDetectorNotFound):
				respondErr(c, http.StatusNotFound, err.Error())
			default:
				respondErr(c, http.StatusServiceUnavailable, err.Error())
			}
			return
		}
		respondOK(c, gin.H{
			"detectorType": c.Param("detectorType"),
			"schema":       schema,
		})
	}
}
