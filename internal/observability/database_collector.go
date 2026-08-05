package observability

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

type DatabaseCollector struct {
	pool              *pgxpool.Pool
	total             *prometheus.Desc
	idle              *prometheus.Desc
	acquired          *prometheus.Desc
	constructing      *prometheus.Desc
	maximum           *prometheus.Desc
	acquires          *prometheus.Desc
	emptyAcquires     *prometheus.Desc
	cancelledAcquires *prometheus.Desc
	acquireTime       *prometheus.Desc
	emptyAcquireWait  *prometheus.Desc
	newConnections    *prometheus.Desc
	maxIdleClosed     *prometheus.Desc
	maxLifetimeClosed *prometheus.Desc
}

func NewDatabaseCollector(pool *pgxpool.Pool) *DatabaseCollector {
	return &DatabaseCollector{
		pool: pool,
		total: prometheus.NewDesc(
			"ryden_db_pool_connections", "Current database pool connections.", nil, nil,
		),
		idle: prometheus.NewDesc(
			"ryden_db_pool_idle_connections", "Current idle database connections.", nil, nil,
		),
		acquired: prometheus.NewDesc(
			"ryden_db_pool_acquired_connections", "Current acquired database connections.", nil, nil,
		),
		constructing: prometheus.NewDesc(
			"ryden_db_pool_constructing_connections", "Connections currently being established.", nil, nil,
		),
		maximum: prometheus.NewDesc(
			"ryden_db_pool_max_connections", "Configured maximum database connections.", nil, nil,
		),
		acquires: prometheus.NewDesc(
			"ryden_db_pool_acquires_total", "Successful database connection acquisitions.", nil, nil,
		),
		emptyAcquires: prometheus.NewDesc(
			"ryden_db_pool_empty_acquires_total", "Acquisitions that had to wait for a connection.", nil, nil,
		),
		cancelledAcquires: prometheus.NewDesc(
			"ryden_db_pool_cancelled_acquires_total", "Connection acquisitions cancelled by context.", nil, nil,
		),
		acquireTime: prometheus.NewDesc(
			"ryden_db_pool_acquire_duration_seconds_total", "Cumulative connection acquisition time.", nil, nil,
		),
		emptyAcquireWait: prometheus.NewDesc(
			"ryden_db_pool_empty_acquire_wait_seconds_total", "Cumulative wait time while the pool had no idle connection.", nil, nil,
		),
		newConnections: prometheus.NewDesc(
			"ryden_db_pool_new_connections_total", "Database connections established by the pool.", nil, nil,
		),
		maxIdleClosed: prometheus.NewDesc(
			"ryden_db_pool_max_idle_closed_total", "Connections closed after exceeding maximum idle time.", nil, nil,
		),
		maxLifetimeClosed: prometheus.NewDesc(
			"ryden_db_pool_max_lifetime_closed_total", "Connections closed after exceeding maximum lifetime.", nil, nil,
		),
	}
}

func (c *DatabaseCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.total
	ch <- c.idle
	ch <- c.acquired
	ch <- c.constructing
	ch <- c.maximum
	ch <- c.acquires
	ch <- c.emptyAcquires
	ch <- c.cancelledAcquires
	ch <- c.acquireTime
	ch <- c.emptyAcquireWait
	ch <- c.newConnections
	ch <- c.maxIdleClosed
	ch <- c.maxLifetimeClosed
}

func (c *DatabaseCollector) Collect(ch chan<- prometheus.Metric) {
	stats := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.total, prometheus.GaugeValue, float64(stats.TotalConns()))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(stats.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.acquired, prometheus.GaugeValue, float64(stats.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.constructing, prometheus.GaugeValue, float64(stats.ConstructingConns()))
	ch <- prometheus.MustNewConstMetric(c.maximum, prometheus.GaugeValue, float64(stats.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.acquires, prometheus.CounterValue, float64(stats.AcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.emptyAcquires, prometheus.CounterValue, float64(stats.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.cancelledAcquires, prometheus.CounterValue, float64(stats.CanceledAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.acquireTime, prometheus.CounterValue, stats.AcquireDuration().Seconds())
	ch <- prometheus.MustNewConstMetric(c.emptyAcquireWait, prometheus.CounterValue, stats.EmptyAcquireWaitTime().Seconds())
	ch <- prometheus.MustNewConstMetric(c.newConnections, prometheus.CounterValue, float64(stats.NewConnsCount()))
	ch <- prometheus.MustNewConstMetric(c.maxIdleClosed, prometheus.CounterValue, float64(stats.MaxIdleDestroyCount()))
	ch <- prometheus.MustNewConstMetric(c.maxLifetimeClosed, prometheus.CounterValue, float64(stats.MaxLifetimeDestroyCount()))
}
