package budget

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func server(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
}

func TestCheck(t *testing.T) {
	s := server(`{"status":"success","data":{"resultType":"vector","result":[{"value":[1,"1500"]}]}}`)
	defer s.Close()

	r, err := New(s.URL, 2000).Check(context.Background())
	if err != nil || !r.Allowed || r.Series != 1500 {
		t.Fatalf("under budget: %+v, %v", r, err)
	}
	r, err = New(s.URL, 1000).Check(context.Background())
	if err != nil || r.Allowed {
		t.Fatalf("over budget must not be allowed: %+v, %v", r, err)
	}
}

func TestCheckDisabledAndFailClosed(t *testing.T) {
	if r, _ := New("http://unused", 0).Check(context.Background()); !r.Allowed {
		t.Fatal("limit 0 disables the check")
	}
	s := server(`{"status":"error","error":"boom"}`)
	defer s.Close()
	if r, err := New(s.URL, 10).Check(context.Background()); err == nil || r.Allowed {
		t.Fatalf("query errors must fail closed: %+v, %v", r, err)
	}
}
