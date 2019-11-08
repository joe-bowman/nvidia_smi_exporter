package main

import (
	"encoding/xml"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const LISTEN_ADDRESS = ":9202"
const NVIDIA_SMI_PATH = "/usr/bin/nvidia-smi"

var (
	testMode      string
	listenAddress string

	nvidia_driver_info = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_driver_info",
		Help: "DriverVersion Information.",
	},
		[]string{"version"},
	)
	nvidia_device_count = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nvidia_device_count",
		Help: "Device Count.",
	})
	nvidia_fanspeed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_fanspeed",
		Help: "Fan speed (rpm).",
	},
		[]string{"minor"},
	)
	nvidia_info = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_info",
		Help: "Device Information.",
	},
		[]string{"minor", "uuid", "productName"},
	)
	nvidia_memory_total = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_memory_total",
		Help: "Total Memory.",
	},
		[]string{"minor"},
	)
	nvidia_memory_used = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_memory_used",
		Help: "Memory in use.",
	},
		[]string{"minor"},
	)
	nvidia_memory_free = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_memory_free",
		Help: "Memory free.",
	},
		[]string{"minor"},
	)
	nvidia_utilization_gpu = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_utilization_gpu",
		Help: "GPU Utilization.",
	},
		[]string{"minor"},
	)
	nvidia_utilization_memory = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_utilization_memory",
		Help: "Memory utilization.",
	},
		[]string{"minor"},
	)
	nvidia_temperatures = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_temperatures",
		Help: "Current temperature.",
	},
		[]string{"minor"},
	)
	nvidia_temperatures_max = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_temperatures_max",
		Help: "Max temperature.",
	},
		[]string{"minor"},
	)
	nvidia_temperatures_slow = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_temperatures_slow",
		Help: "Throttle temperature.",
	},
		[]string{"minor"},
	)
	nvidia_power_usage = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_power_usage",
		Help: "Current power consumption.",
	},
		[]string{"minor"},
	)
	nvidia_power_limit = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_power_limit",
		Help: "Max power consumption.",
	},
		[]string{"minor"},
	)
	nvidia_clock_graphics = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_clock_graphics",
		Help: "Current graphics clock frequency.",
	},
		[]string{"minor"},
	)
	nvidia_clock_graphics_max = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_clock_graphics_max",
		Help: "Max graphics clock frequency.",
	},
		[]string{"minor"},
	)
	nvidia_clock_sm = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_clock_sm",
		Help: "Current SM clock frequency.",
	},
		[]string{"minor"},
	)
	nvidia_clock_sm_max = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_clock_sm_max",
		Help: "Max graphics SM frequency.",
	},
		[]string{"minor"},
	)
	nvidia_clock_mem = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_clock_mem",
		Help: "Current DRAM clock frequency.",
	},
		[]string{"minor"},
	)
	nvidia_clock_mem_max = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_clock_mem_max",
		Help: "Max DRAM clock frequency.",
	},
		[]string{"minor"},
	)
	nvidia_clock_video = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_clock_video",
		Help: "Current video clock frequency.",
	},
		[]string{"minor"},
	)
	nvidia_clock_video_max = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nvidia_clock_video_max",
		Help: "Max video clock frequency.",
	},
		[]string{"minor"},
	)
)

type NvidiaSmiLog struct {
	DriverVersion string `xml:"driver_version"`
	AttachedGPUs  string `xml:"attached_gpus"`
	GPUs          []struct {
		ProductName  string `xml:"product_name"`
		ProductBrand string `xml:"product_brand"`
		UUID         string `xml:"uuid"`
		FanSpeed     string `xml:"fan_speed"`
		PCI          struct {
			PCIBus string `xml:"pci_bus"`
		} `xml:"pci"`
		FbMemoryUsage struct {
			Total string `xml:"total"`
			Used  string `xml:"used"`
			Free  string `xml:"free"`
		} `xml:"fb_memory_usage"`
		Utilization struct {
			GPUUtil    string `xml:"gpu_util"`
			MemoryUtil string `xml:"memory_util"`
		} `xml:"utilization"`
		Temperature struct {
			GPUTemp              string `xml:"gpu_temp"`
			GPUTempMaxThreshold  string `xml:"gpu_temp_max_threshold"`
			GPUTempSlowThreshold string `xml:"gpu_temp_slow_threshold"`
		} `xml:"temperature"`
		PowerReadings struct {
			PowerDraw  string `xml:"power_draw"`
			PowerLimit string `xml:"power_limit"`
		} `xml:"power_readings"`
		Clocks struct {
			GraphicsClock string `xml:"graphics_clock"`
			SmClock       string `xml:"sm_clock"`
			MemClock      string `xml:"mem_clock"`
			VideoClock    string `xml:"video_clock"`
		} `xml:"clocks"`
		MaxClocks struct {
			GraphicsClock string `xml:"graphics_clock"`
			SmClock       string `xml:"sm_clock"`
			MemClock      string `xml:"mem_clock"`
			VideoClock    string `xml:"video_clock"`
		} `xml:"max_clocks"`
	} `xml:"gpu"`
}

func filterNumber(value string) float64 {
	r := regexp.MustCompile("[^0-9.]")
	value = r.ReplaceAllString(value, "")
	if value == "" {
		return -1.0
	}
	retval, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Fatal(err)
	}
	return retval
}

func metrics() {

	go func() {
		for {
			var cmd *exec.Cmd
			if testMode == "1" {
				dir, err := os.Getwd()
				if err != nil {
					log.Fatal(err)
				}
				cmd = exec.Command("/bin/cat", dir+"/test.xml")
			} else {
				cmd = exec.Command(NVIDIA_SMI_PATH, "-q", "-x")
			}

			// Execute system command
			stdout, err := cmd.Output()
			if err != nil {
				println(err.Error())
				return
			}
			log.Println("Querying SMI...")

			// Parse XML
			var xmlData NvidiaSmiLog
			xml.Unmarshal(stdout, &xmlData)

			// Output
			nvidia_driver_info.With(prometheus.Labels{"version": xmlData.DriverVersion}).Set(1)
			nvidia_device_count.Set(filterNumber(xmlData.AttachedGPUs))
			for i, GPU := range xmlData.GPUs {
				var idx = strconv.Itoa(i)
				nvidia_fanspeed.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.FanSpeed))
				nvidia_info.With(prometheus.Labels{"minor": idx, "uuid": GPU.UUID, "productName": GPU.ProductName}).Set(1)
				nvidia_memory_total.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.FbMemoryUsage.Total))
				nvidia_memory_used.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.FbMemoryUsage.Used))
				nvidia_memory_free.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.FbMemoryUsage.Free))
				nvidia_utilization_gpu.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.Utilization.GPUUtil))
				nvidia_utilization_memory.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.Utilization.MemoryUtil))
				nvidia_temperatures.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.Temperature.GPUTemp))
				nvidia_temperatures_max.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.Temperature.GPUTempMaxThreshold))
				nvidia_temperatures_slow.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.Temperature.GPUTempSlowThreshold))
				nvidia_power_usage.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.PowerReadings.PowerDraw))
				nvidia_power_limit.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.PowerReadings.PowerLimit))
				nvidia_clock_graphics.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.Clocks.GraphicsClock))
				nvidia_clock_graphics_max.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.MaxClocks.GraphicsClock))
				nvidia_clock_sm.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.Clocks.SmClock))
				nvidia_clock_sm_max.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.MaxClocks.SmClock))
				nvidia_clock_mem.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.Clocks.MemClock))
				nvidia_clock_mem_max.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.MaxClocks.MemClock))
				nvidia_clock_video.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.Clocks.VideoClock))
				nvidia_clock_video_max.With(prometheus.Labels{"minor": idx}).Set(filterNumber(GPU.MaxClocks.VideoClock))
			}

			time.Sleep(5 * time.Second)
		}
	}()
}

func index(w http.ResponseWriter, r *http.Request) {
	log.Print("Serving /index")
	html := `<!doctype html>
<html>
    <head>
        <meta charset="utf-8">
        <title>Nvidia SMI Exporter</title>
    </head>
    <body>
        <h1>Nvidia SMI Exporter</h1>
        <p><a href="/metrics">Metrics</a></p>
    </body>
</html>`
	io.WriteString(w, html)
}

func main() {

	testMode = os.Getenv("TEST_MODE")
	if testMode == "1" {
		log.Print("Test mode is enabled")
	}

	listenAddress = os.Getenv("LISTEN_ADDRESS")
	if len(listenAddress) == 0 {
		listenAddress = LISTEN_ADDRESS
	}

	log.Print("Nvidia SMI exporter listening on " + listenAddress)
	metrics()
	http.HandleFunc("/", index)

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(LISTEN_ADDRESS, nil))
}
