package collect

import "database/sql"

// metricsDB is set by api.NewRouter so collect can query ops_metrics
// without importing api (avoids an import cycle).
var metricsDB *sql.DB

// SetMetricsDB injects the shared pgsql pool for metric queries.
func SetMetricsDB(d *sql.DB) { metricsDB = d }

// queryMetrics loads cpu/mem samples newer than cutoff (unix seconds)
// from the ops_metrics table. Returns nil slices when unavailable.
func queryMetrics(cutoff int64) (cpu, mem []Sample) {
	if metricsDB == nil {
		return nil, nil
	}
	rows, err := metricsDB.Query(
		`SELECT metric, extract(epoch from time)::bigint, value FROM ops_metrics
		 WHERE time > to_timestamp($1) AND metric IN ('cpu','mem') ORDER BY time`, cutoff)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()
	for rows.Next() {
		var metric string
		var t int64
		var v float64
		if err := rows.Scan(&metric, &t, &v); err != nil {
			continue
		}
		switch metric {
		case "cpu":
			cpu = append(cpu, Sample{T: t, V: v})
		case "mem":
			mem = append(mem, Sample{T: t, V: v})
		}
	}
	return cpu, mem
}
