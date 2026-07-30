package inventorysvc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// The handler is wired directly rather than through RegisterRoutes so the test
// does not need a token manager or a role checker. The Service holds a nil
// database, which is safe here because rejected input never reaches a query - a
// case that did would panic instead of passing quietly.
func stockLogRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/stock-logs", (&Handler{service: NewService(nil)}).listLogs)
	return router
}

func TestListLogsRejectsMalformedProductID(t *testing.T) {
	router := stockLogRouter()

	for _, raw := range []string{"abc", "1.5", "-1", "9223372036854775808"} {
		t.Run(raw, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/stock-logs?product_id="+raw, nil))

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", recorder.Code)
			}

			var body struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.Code != 40014 {
				t.Fatalf("expected code 40014, got %d (%s)", body.Code, body.Msg)
			}
		})
	}
}

// An absent or zero product_id means "every product", which is what the admin
// console sends for its 全部商品 option. Reaching the database proves the
// handler treated the value as a request for everything rather than rejecting
// it: with a nil database that surfaces as a panic, which is caught here.
func TestListLogsTreatsEmptyProductIDAsNoFilter(t *testing.T) {
	router := stockLogRouter()

	for _, query := range []string{"", "?product_id=", "?product_id=0", "?product_id=%20"} {
		t.Run("query="+query, func(t *testing.T) {
			reachedDatabase := false
			func() {
				defer func() {
					if recover() != nil {
						reachedDatabase = true
					}
				}()
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/stock-logs"+query, nil))
				if recorder.Code == http.StatusBadRequest {
					t.Fatalf("expected the request to be accepted, got 400: %s", recorder.Body.String())
				}
			}()
			if !reachedDatabase {
				t.Fatal("expected the handler to query the database rather than reject the request")
			}
		})
	}
}
