package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jsgoecke/nest"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Config struct {
	ClientID     string `toml:"client_id"`
	ClientSecret string `toml:"client_secret"`
	Token        string
}

// Prometheus stats
var (
	stateStat = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nest_state",
			Help: "Various true/false (1/0) metrics decribing nest state",
		},
		[]string{"thermostat", "property"},
	)
	tempStat = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nest_temperature",
			Help: "The ambient temperature in F",
		},
		[]string{"thermostat"},
	)
	targetTempStat = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nest_target_temperature",
			Help: "The target temperatures in F",
		},
		[]string{"thermostat", "type"},
	)
	humidityStat = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nest_humidity",
			Help: "Current humidity in %",
		},
		[]string{"thermostat"},
	)
	hvacModeStat = prometheus.NewGaugeVec(
		// heat, cool, heat-cool, eco, off
		prometheus.GaugeOpts{
			Name: "nest_hvac_mode",
			Help: "HVAC mode",
		},
		[]string{"thermostat", "mode"},
	)
	hvacStateStat = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nest_hvac_state",
			Help: "HVAC state",
		},
		[]string{"thermostat", "state"},
	)
)

// Flags
var (
	addr = flag.String("listen-address", ":9264",
		"The address to listen on for HTTP requests.")
	configFile = flag.String("config", "~/.nest_exporter.toml",
		"Path to the configuration file.")
)

func init() {
	flag.Parse()
	prometheus.MustRegister(stateStat)
	prometheus.MustRegister(tempStat)
	prometheus.MustRegister(targetTempStat)
	prometheus.MustRegister(humidityStat)
	prometheus.MustRegister(hvacModeStat)
	prometheus.MustRegister(hvacStateStat)
}

func main() {
	var config Config
	if _, err := toml.DecodeFile(*configFile, &config); err != nil {
		log.Fatal(err)
	}
	client := nest.New(config.ClientID, "STATE", config.ClientSecret, "")
	client.Token = config.Token
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for {
			devices, err := client.Devices()
			if err != nil {
				errContent, jsonErr := json.Marshal(err)
				if jsonErr != nil {
					log.Fatal("Unknown API Error")
				}
				log.Println("API Error:", string(errContent))
			} else {
				for _, t := range devices.Thermostats {
					// States
					var isOnline float64
					var canCool float64
					var canHeat float64
					var isUsingEmergencyHeat float64
					var hasFan float64
					var fanTimerActive float64
					var hasLeaf float64

					if t.IsOnline {
						isOnline = 1
					}
					if t.CanCool {
						canCool = 1
					}
					if t.CanHeat {
						canHeat = 1
					}
					if t.IsUsingEmergencyHeat {
						isUsingEmergencyHeat = 1
					}
					if t.HasFan {
						hasFan = 1
					}
					if t.FanTimerActive {
						fanTimerActive = 1
					}
					if t.HasLeaf {
						hasLeaf = 1
					}
					stateStat.With(prometheus.Labels{
						"thermostat": t.Name, "property": "is_online",
					}).Set(isOnline)
					stateStat.With(prometheus.Labels{
						"thermostat": t.Name, "property": "can_cool",
					}).Set(canCool)
					stateStat.With(prometheus.Labels{
						"thermostat": t.Name, "property": "can_heat",
					}).Set(canHeat)
					stateStat.With(prometheus.Labels{
						"thermostat": t.Name,
						"property":   "is_using_emergency_heat",
					}).Set(isUsingEmergencyHeat)
					stateStat.With(prometheus.Labels{
						"thermostat": t.Name, "property": "has_fan",
					}).Set(hasFan)
					stateStat.With(prometheus.Labels{
						"thermostat": t.Name, "property": "fan_timer_active",
					}).Set(fanTimerActive)
					stateStat.With(prometheus.Labels{
						"thermostat": t.Name, "property": "has_leaf",
					}).Set(hasLeaf)

					// Ambient Temperature
					tempStat.With(prometheus.Labels{
						"thermostat": t.Name,
					}).Set(float64(t.AmbientTemperatureF))

					// Target Temperatures
					targetTempStat.Reset()
					tts := targetTempStat.MustCurryWith(prometheus.Labels{
						"thermostat": t.Name,
					})

					if t.HvacMode == "heat" || t.HvacMode == "cool" {
						tts.With(prometheus.Labels{
							"type": "target_temperature",
						}).Set(float64(t.TargetTemperatureF))
					} else if t.HvacMode == "heat-cool" {
						tts.With(prometheus.Labels{
							"type": "target_temperature_high",
						}).Set(float64(t.TargetTemperatureHighF))
						tts.With(prometheus.Labels{
							"type": "target_temperature_low",
						}).Set(float64(t.TargetTemperatureLowF))
					} else if t.HvacMode == "eco" {
						tts.With(prometheus.Labels{
							"type": "away_temperature_high",
						}).Set(float64(t.AwayTemperatureHighF))
						tts.With(prometheus.Labels{
							"type": "away_temperature_low",
						}).Set(float64(t.AwayTemperatureLowF))
					}

					// The hvac state and mode stats are implemented by having
					// a metric with the label of the current hvac state.
					// Other states/modes aren't present.
					hvacStateStat.Reset() // Remove the previous hvac state
					hvacStateStat.With(prometheus.Labels{
						"thermostat": t.Name, "state": t.HvacState,
					}).Set(1.0)

					hvacModeStat.Reset() // Remove the previous hvac mode
					hvacModeStat.With(prometheus.Labels{
						"thermostat": t.Name, "mode": t.HvacMode,
					}).Set(1.0)

					humidityStat.With(
						prometheus.Labels{"thermostat": t.Name}).Set(
						float64(t.Humidity))
				}
			}
			// Wait until the next tick
			<-ticker.C
		}
	}()

	log.Println("Listening on", *addr)
	http.Handle("/metrics", promhttp.Handler())
	http.Handle("/", http.RedirectHandler("/metrics", 302))
	log.Fatal(http.ListenAndServe(*addr, nil))
}
