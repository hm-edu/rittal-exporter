package main

import (
	"context"
	"github.com/gosnmp/gosnmp"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Collector struct {
	Devices    map[string]RittalDevice
	Ip         string
	Community  string
	DeviceType string
}

type lookup struct {
	device   string
	variable string
	item     string
}

func (c Collector) Describe(descs chan<- *prometheus.Desc) {
	descs <- prometheus.NewDesc("rittal_value", "", []string{}, prometheus.Labels{})
	descs <- prometheus.NewDesc("rittal_status", "", []string{}, prometheus.Labels{})
}

func (c Collector) Collect(metrics chan<- prometheus.Metric) {
	snmp := gosnmp.GoSNMP{}
	snmp.Context = context.Background()
	snmp.Community = c.Community
	snmp.Version = gosnmp.Version2c
	snmp.Target = c.Ip
	snmp.Port = 161
	snmp.Transport = "udp"
	snmp.Timeout = 30 * time.Second
	snmp.MaxRepetitions = 50
	err := snmp.Connect()
	if err != nil {
		return
	}
	defer snmp.Conn.Close()

	valueOids := map[string]lookup{}
	statusOids := map[string]lookup{}
	re := regexp.MustCompile(`-?\d[\d,]*[.]?[\d{2}]*`)
	socket := regexp.MustCompile(`Sockets\.(Socket \d+)\.([^.]+)\..*`)
	airTemp := regexp.MustCompile(`Air Temp.(Server (Out|In).(\w+))`)
	fan := regexp.MustCompile(`Fans.Current Speed.(Fan\d)`)
	remote := regexp.MustCompile(`(Fans|Control Valve)\.Remote`)
	status := regexp.MustCompile(`(.*) Status$`)
	value := regexp.MustCompile(`(.*) Value$`)
	for dev, item := range c.Devices {
		for _, obj := range item.Variables {
			if obj.Description == "" {
				continue
			}
			name := ""
			item := ""
			s := socket.FindStringSubmatch(obj.RawTitle)
			if len(s) > 0 {
				item = s[1]
				name = s[2]
			}
			s = airTemp.FindStringSubmatch(obj.RawTitle)
			if len(s) > 0 {
				item = s[2]
				name = "Temperature " + s[3]
			}
			s = fan.FindStringSubmatch(obj.RawTitle)
			if len(s) > 0 {
				item = s[1]
				name = "Speed"
			}
			s = remote.FindStringSubmatch(obj.RawTitle)
			if len(s) > 0 {
				item = s[0]
			}
			for _, x := range obj.ValueOids {
				if item == "" {
					name = obj.Description + " " + x.Name
					if value.MatchString(name) {
						name = value.FindStringSubmatch(name)[1]
					}
				}
				valueOids[x.Oid] = lookup{device: dev, variable: name, item: item}
			}
			for _, x := range obj.StatusOids {
				if item == "" {
					name = obj.Description + " " + x.Name
					if status.MatchString(name) {
						name = status.FindStringSubmatch(name)[1]
					}
				}
				statusOids[x.Oid] = lookup{device: dev, variable: name, item: item}
			}
		}
	}
	logger, _ := zap.NewProduction()
	logger.Debug("Scraping ...")
	oids := []string{}
	for key := range valueOids {
		oids = append(oids, key)
	}
	data, err := loadSnmpData(snmp, oids)
	if err != nil {
		return
	}
	seenValues := map[string]float64{}
	seenStatus := map[string]int{}
	for _, v := range data {
		bytes := v.Value.([]byte)
		match := re.Find(bytes)
		if match != nil {
			if s, err := strconv.ParseFloat(string(match), 64); err == nil {
				labels := []string{"device", "variable", "type"}
				labelValues := []string{valueOids[strings.TrimPrefix(v.Name, ".")].device, valueOids[strings.TrimPrefix(v.Name, ".")].variable, c.DeviceType}
				if s := valueOids[strings.TrimPrefix(v.Name, ".")].item; s != "" {
					labels = append(labels, "item")
					labelValues = append(labelValues, s)
				}
				if _, ok := seenValues[strings.Join(labelValues, "-")]; !ok {
					seenValues[strings.Join(labelValues, "-")] = s
					metrics <- prometheus.MustNewConstMetric(
						prometheus.NewDesc("rittal_value", "", labels, nil),
						prometheus.GaugeValue,
						s,
						labelValues...,
					)
				}
			}

		}
	}
	oids = []string{}
	for key := range statusOids {
		oids = append(oids, key)
	}
	data, err = loadSnmpData(snmp, oids)
	if err != nil {
		return
	}
	for _, v := range data {
		value := v.Value.(int)
		if err == nil {
			labels := []string{"device", "variable", "type"}
			labelValues := []string{statusOids[strings.TrimPrefix(v.Name, ".")].device, statusOids[strings.TrimPrefix(v.Name, ".")].variable, c.DeviceType}
			if s := statusOids[strings.TrimPrefix(v.Name, ".")].item; s != "" {
				labels = append(labels, "item")
				labelValues = append(labelValues, s)
			}
			if _, ok := seenStatus[strings.Join(labelValues, "-")]; !ok {
				seenStatus[strings.Join(labelValues, "-")] = value
				metrics <- prometheus.MustNewConstMetric(
					prometheus.NewDesc("rittal_status", "Value Mapping: 1 -> not available 2 -> configuration changed 3 -> error 4 -> OK 5 -> alarm 6 -> warning value reached high warning threshold 7 -> alarm value reached low threshold 8 -> alarm value reached high threshold 9 -> warning value reached low warning threshold 10 -> output OFF 11 -> output ON 12 -> door open 13 -> door closed 14 -> door locked 15 -> door unlocked remote input 16 -> door unlocked reader or keypad 17 -> door unlocked SNMP set 18 -> door unlocked WEB 19 -> door unlocked timer 20 -> no access 21 -> orientation PSM unit circuit 1 22 -> orientation PSM unit circuit 2 23 -> battery low, wireless sensor 24 -> sensor cable broken 25 -> sensor cable short 26 -> sensor calibration in progress 27 -> sensor inactive 28 -> sensor active 29 -> no Power (PSM)", labels, nil),
					prometheus.GaugeValue,
					float64(value),
					labelValues...,
				)
			}
		}

	}
}
