package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type accountTransportHealthUpstreamStub struct {
	service.HTTPUpstream
	snapshot service.AccountTransportHealthSnapshot
}

func (s *accountTransportHealthUpstreamStub) SnapshotTransportHealth(_ int64) service.AccountTransportHealthSnapshot {
	return s.snapshot
}

func TestAccountHandlerGetTransportHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AccountHandler{
		httpUpstream: &accountTransportHealthUpstreamStub{
			snapshot: service.AccountTransportHealthSnapshot{
				AccountID:         42,
				RequestsTotal:     3,
				HTTP2SuccessTotal: 2,
			},
		},
	}
	router := gin.New()
	router.GET("/accounts/:id/transport-health", h.GetTransportHealth)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/accounts/42/transport-health", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope struct {
		Code int                                    `json:"code"`
		Data service.AccountTransportHealthSnapshot `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)
	require.Equal(t, int64(42), envelope.Data.AccountID)
	require.Equal(t, int64(2), envelope.Data.HTTP2SuccessTotal)
}

func TestAccountHandlerGetTransportHealthRejectsInvalidID(t *testing.T) {
	h := &AccountHandler{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Params = gin.Params{{Key: "id", Value: "0"}}
	h.GetTransportHealth(c)
	require.Equal(t, http.StatusBadRequest, c.Writer.Status())
}
