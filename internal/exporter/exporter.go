package exporter

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/showwin/speedtest-go/speedtest"
	log "github.com/sirupsen/logrus"
)

const (
	namespace = "speedtest"
)

var (
	up = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Was the last speedtest successful.",
		[]string{"test_uuid"}, nil,
	)
	scrapeDurationSeconds = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "scrape_duration_seconds"),
		"Time to preform last speed test",
		[]string{"test_uuid"}, nil,
	)
	latency = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "latency_seconds"),
		"Measured latency on last speed test",
		[]string{"test_uuid", "user_lat", "user_lon", "user_ip", "user_isp", "server_lat", "server_lon", "server_id", "server_name", "server_country", "distance"},
		nil,
	)
	upload = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "upload_speed_Bps"),
		"Last upload speedtest result",
		[]string{"test_uuid", "user_lat", "user_lon", "user_ip", "user_isp", "server_lat", "server_lon", "server_id", "server_name", "server_country", "distance"},
		nil,
	)
	download = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "download_speed_Bps"),
		"Last download speedtest result",
		[]string{"test_uuid", "user_lat", "user_lon", "user_ip", "user_isp", "server_lat", "server_lon", "server_id", "server_name", "server_country", "distance"},
		nil,
	)
)

// Exporter runs speedtest and exports them using
// the prometheus metrics package.
type Exporter struct {
	serverID       int
	serverFallback bool
}

// New returns an initialized Exporter.
func New(serverID int, serverFallback bool) (*Exporter, error) {
	return &Exporter{
		serverID:       serverID,
		serverFallback: serverFallback,
	}, nil
}

// Describe describes all the metrics. It implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- up
	ch <- scrapeDurationSeconds
	ch <- latency
	ch <- upload
	ch <- download
}

// Collect fetches the stats from Starlink dish and delivers them
// as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	testUUID := uuid.New().String()
	start := time.Now()
	ok := e.speedtest(testUUID, ch)

	if ok {
		ch <- prometheus.MustNewConstMetric(
			up, prometheus.GaugeValue, 1.0,
			testUUID,
		)
		ch <- prometheus.MustNewConstMetric(
			scrapeDurationSeconds, prometheus.GaugeValue, time.Since(start).Seconds(),
			testUUID,
		)
	} else {
		ch <- prometheus.MustNewConstMetric(
			up, prometheus.GaugeValue, 0.0,
			testUUID,
		)
	}
}

func (e *Exporter) speedtest(testUUID string, ch chan<- prometheus.Metric) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	user, err := speedtest.FetchUserInfoContext(ctx)
	if err != nil {
		log.Errorf("could not fetch user information: %s", err.Error())
		return false
	}

	serverList, err := speedtest.FetchServerListContext(ctx)
	if err != nil {
		log.Errorf("could not fetch server list: %s", err.Error())
		return false
	}

	var server *speedtest.Server

	servers, err := serverList.FindServer([]int{e.serverID})
	if err != nil {
		log.Error(fmt.Sprintf("could not find server ID %d: %s", e.serverID, err.Error()))
		return false
	}

	server = servers[0]

	ok := pingTest(ctx, testUUID, user, server, ch)
	ok = downloadTest(ctx, testUUID, user, server, ch) && ok
	ok = uploadTest(ctx, testUUID, user, server, ch) && ok

	return ok
}

func pingTest(ctx context.Context, testUUID string, user *speedtest.User, server *speedtest.Server, ch chan<- prometheus.Metric) bool {
	var measurements []float64

	err := server.PingTestContext(ctx, func(d time.Duration) {
		measurements = append(measurements, d.Seconds())
	})

	if err != nil {
		log.Errorf("failed to carry out ping test: %s", err.Error())
		return false
	}

	if len(measurements) > 0 {
		var sum float64
		for _, m := range measurements {
			sum += m
		}
		averageLatency := sum / float64(len(measurements))

		ch <- prometheus.MustNewConstMetric(
			latency,
			prometheus.GaugeValue,
			averageLatency,
			testUUID,
			user.Lat,
			user.Lon,
			user.IP,
			user.Isp,
			server.Lat,
			server.Lon,
			server.ID,
			server.Name,
			server.Country,
			fmt.Sprintf("%f", server.Distance),
		)
	}

	return true
}

func downloadTest(ctx context.Context, testUUID string, user *speedtest.User, server *speedtest.Server, ch chan<- prometheus.Metric) bool {
	err := server.DownloadTestContext(ctx)
	if err != nil {
		log.Errorf("failed to carry out download test: %s", err.Error())
		return false
	}

	ch <- prometheus.MustNewConstMetric(
		download,
		prometheus.GaugeValue,
		float64(server.DLSpeed)*12.5,
		testUUID,
		user.Lat,
		user.Lon,
		user.IP,
		user.Isp,
		server.Lat,
		server.Lon,
		server.ID,
		server.Name,
		server.Country,
		fmt.Sprintf("%f", server.Distance),
	)

	return true
}

func uploadTest(ctx context.Context, testUUID string, user *speedtest.User, server *speedtest.Server, ch chan<- prometheus.Metric) bool {
	err := server.UploadTestContext(ctx)
	if err != nil {
		log.Errorf("failed to carry out upload test: %s", err.Error())
		return false
	}

	ch <- prometheus.MustNewConstMetric(
		upload,
		prometheus.GaugeValue,
		float64(server.ULSpeed)*12.5,
		testUUID,
		user.Lat,
		user.Lon,
		user.IP,
		user.Isp,
		server.Lat,
		server.Lon,
		server.ID,
		server.Name,
		server.Country,
		fmt.Sprintf("%f", server.Distance),
	)

	return true
}
