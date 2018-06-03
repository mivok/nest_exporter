package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Config struct {
	Token           string
	RefreshInterval int `toml:"refresh_interval"`
}

type Devices struct {
	Thermostats map[string]Device
}

type Device struct {
	Humidity                  float64
	Locale                    string
	TemperatureScale          string `json:"temperature_scale"`
	IsUsingEmergencyHeat      bool   `json:"is_using_emergency_heat"`
	HasFan                    bool   `json:"has_fan"`
	SoftwareVersion           string `json:"software_version"`
	HasLeaf                   bool   `json:"has_leaf"`
	WhereId                   string `json:"where_id"`
	DeviceId                  string `json:"device_id"`
	Name                      string
	CanHeat                   bool    `json:"can_heat"`
	CanCool                   bool    `json:"can_cool"`
	TargetTemperatureC        float64 `json:"target_temperature_c"`
	TargetTemperatureF        float64 `json:"target_temperature_f"`
	TargetTemperatureHighC    float64 `json:"target_temperature_high_c"`
	TargetTemperatureHighF    float64 `json:"target_temperature_high_f"`
	TargetTemperatureLowC     float64 `json:"target_temperature_low_c"`
	TargetTemperatureLowF     float64 `json:"target_temperature_low_f"`
	AmbientTemperatureC       float64 `json:"ambient_temperature_c"`
	AmbientTemperatureF       float64 `json:"ambient_temperature_f"`
	AwayTemperatureHighC      float64 `json:"away_temperature_high_c"`
	AwayTemperatureHighF      float64 `json:"away_temperature_high_f"`
	AwayTemperatureLowC       float64 `json:"away_temperature_low_c"`
	AwayTemperatureLowF       float64 `json:"away_temperature_low_f"`
	EcoTemperatureHighC       float64 `json:"eco_temperature_high_c"`
	EcoTemperatureHighF       float64 `json:"eco_temperature_high_f"`
	EcoTemperatureLowC        float64 `json:"eco_temperature_low_c"`
	EcoTemperatureLowF        float64 `json:"eco_temperature_low_f"`
	IsLocked                  bool    `json:"is_locked"`
	LockedTempMinC            float64 `json:_min_c"`
	LockedTempMinF            float64 `json:"locked_temp_min_f"`
	LockedTempMaxC            float64 `json:"locked_temp_max_c"`
	LockedTempMaxF            float64 `json:"locked_temp_max_f"`
	SunlightCorrectionActive  bool    `json:"sunlight_correction_active"`
	SunlightCorrectionEnabled bool    `json:"sunlight_correction_enabled"`
	StructureId               string  `json:"structure_id"`
	FanTimerActive            bool    `json:"fan_timer_active"`
	FanTimerTimeout           string  `json:"fan_timer_timeout"`
	FanTimerDuration          float64 `json:"fan_timer_duration"`
	PreviousHvacMode          string  `json:"previous_hvac_mode"`
	HvacMode                  string  `json:"hvac_mode"`
	TimeToTarget              string  `json:"time_to_target"`
	TimeToTargetTraining      string  `json:"time_to_target_training"`
	WhereName                 string  `json:"where_name"`
	Label                     string
	NameLong                  string `json:"name_long"`
	IsOnline                  bool   `json:"is_online"`
	LastConnection            string `json:"last_connection"`
	HvacState                 string `json:"hvac_state"`
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

// Other
var (
	CachedRedirectURL string // because nest wants you to reuse the redirect URL
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

func getDevices(token string) (*Devices, error) {
	// See https://developers.nest.com/documentation/cloud/how-to-handle-redirects#store_the_redirected_location
	// for why there is a cached redirect URL
	url := CachedRedirectURL
	if url == "" {
		url = "https://developer-api.nest.com/devices.json/?auth=" + token
	}
	resp, err := http.Get(url)
	respURL := resp.Request.URL.String()
	if respURL != url {
		// We were redirected, so cache the new URL
		CachedRedirectURL = respURL
	}
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP code %d: %s", resp.StatusCode, body)
	}
	devices := &Devices{}
	err = json.Unmarshal(body, devices)
	if err != nil {
		return nil, err
	}
	return devices, nil
}

func main() {
	var config Config
	if _, err := toml.DecodeFile(*configFile, &config); err != nil {
		log.Fatal(err)
	}
	if config.RefreshInterval == 0 {
		// Default to 2 minute refreshes
		config.RefreshInterval = 120
	}
	ticker := time.NewTicker(time.Duration(config.RefreshInterval) *
		time.Second)
	go func() {
		for {
			devices, err := getDevices(config.Token)
			if err != nil {
				log.Println(err)
			} else {
				for _, t := range devices.Thermostats {
					// States - 1 for on, 0 for off
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
						}).Set(t.TargetTemperatureF)
					} else if t.HvacMode == "heat-cool" {
						tts.With(prometheus.Labels{
							"type": "target_temperature_high",
						}).Set(t.TargetTemperatureHighF)
						tts.With(prometheus.Labels{
							"type": "target_temperature_low",
						}).Set(t.TargetTemperatureLowF)
					} else if t.HvacMode == "eco" {
						tts.With(prometheus.Labels{
							"type": "away_temperature_high",
						}).Set(t.AwayTemperatureHighF)
						tts.With(prometheus.Labels{
							"type": "away_temperature_low",
						}).Set(t.AwayTemperatureLowF)
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
						t.Humidity)
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
