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

	log.Info("collect 1")

	if ok {
		log.Info("collect 2")
		ch <- prometheus.MustNewConstMetric(
			up, prometheus.GaugeValue, 1.0,
			testUUID,
		)
		log.Info("collect 3")
		ch <- prometheus.MustNewConstMetric(
			scrapeDurationSeconds, prometheus.GaugeValue, time.Since(start).Seconds(),
			testUUID,
		)
		log.Info("collect 4")
	} else {
		log.Info("collect 5")
		ch <- prometheus.MustNewConstMetric(
			up, prometheus.GaugeValue, 0.0,
			testUUID,
		)
	}
}

func (e *Exporter) speedtest(testUUID string, ch chan<- prometheus.Metric) bool {
	log.Info("speedtest 1")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	log.Info("speedtest 2")
	user, err := speedtest.FetchUserInfoContext(ctx)
	if err != nil {
		log.Errorf("could not fetch user information: %s", err.Error())
		return false
	}
	log.Info("speedtest 3")
	// returns list of servers in distance order
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
	log.Info("speedtest 4")
	// if servers[0].ID != fmt.Sprintf("%d", e.serverID) && !e.serverFallback {
	// 	log.Errorf("could not find your choosen server ID %d in the list of avaiable servers, server_fallback is not set so failing this test", e.serverID)
	// 	return false
	// }

	server = servers[0]

	ok := pingTest(ctx, testUUID, user, server, ch)
	ok = downloadTest(ctx, testUUID, user, server, ch) && ok
	ok = uploadTest(ctx, testUUID, user, server, ch) && ok

	return ok
}

func pingTest(ctx context.Context, testUUID string, user *speedtest.User, server *speedtest.Server, ch chan<- prometheus.Metric) bool {
	log.Info("pingTest 1")
	var measurements []float64

	err := server.PingTestContext(ctx, func(d time.Duration) {
		log.Info("pingTest 2")
		measurements = append(measurements, d.Seconds())
	})

	log.Info("pingTest 3")
	if err != nil {
		log.Errorf("failed to carry out ping test: %s", err.Error())
		return false
	}

	// Calculate average latency after all measurements are collected
	if len(measurements) > 0 {
		var sum float64
		for _, m := range measurements {
			sum += m
		}
		averageLatency := sum / float64(len(measurements))

		// Emit single metric with average latency
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

	log.Info("pingTest 4")
	return true
}

func downloadTest(ctx context.Context, testUUID string, user *speedtest.User, server *speedtest.Server, ch chan<- prometheus.Metric) bool {
	log.Info("downloadTest 1")
	err := server.DownloadTestContext(ctx)
	if err != nil {
		log.Errorf("failed to carry out download test: %s", err.Error())
		return false
	}
	log.Info("downloadTest 2")
	// server.DLSpeed is in Mbps (megabits per second)
	// Convert to bytes per second: Mbps * 125000 (1000000/8)
	ch <- prometheus.MustNewConstMetric(
		download,
		prometheus.GaugeValue,
		float64(server.DLSpeed)*125000,
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
	log.Info("downloadTest 3")
	return true
}

func uploadTest(ctx context.Context, testUUID string, user *speedtest.User, server *speedtest.Server, ch chan<- prometheus.Metric) bool {
	log.Info("uploadTest 1")
	err := server.UploadTestContext(ctx)
	if err != nil {
		log.Errorf("failed to carry out upload test: %s", err.Error())
		return false
	}
	log.Info("uploadTest 2")
	// server.ULSpeed is in Mbps (megabits per second)
	// Convert to bytes per second: Mbps * 125000 (1000000/8)
	ch <- prometheus.MustNewConstMetric(
		upload,
		prometheus.GaugeValue,
		float64(server.ULSpeed)*125000,
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
	log.Info("uploadTest 3")
	return true
}
