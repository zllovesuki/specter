package metrics

import (
	"net/http"
	"sync"

	"github.com/VictoriaMetrics/metrics"
)

var (
	metricSet = metrics.NewSet()
	histoMap  = sync.Map{}
)

func MetricsHandler(w http.ResponseWriter, _ *http.Request) {
	metricSet.WritePrometheus(w)
	metrics.WriteProcessMetrics(w)
}
