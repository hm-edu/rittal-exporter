package main

import (
	"context"
	"fmt"
	"github.com/gosnmp/gosnmp"
	"strconv"
	"strings"
	"time"
)

type Target struct {
	Alias string
	Host  string
	Type  string
}

type RittalVariable struct {
	Device     int
	Variable   int
	Name       string
	Type       int
	DeviceName string
}

type SubObject struct {
	Name string
	Oid  string
}

type QueryObject struct {
	Description string
	RawTitle    string
	StatusOids  []SubObject
	ValueOids   []SubObject
}

type RittalDevice struct {
	Variables map[string]*QueryObject
}

func loadDevices(target string, community string) (map[string]RittalDevice, error) {
	snmp := gosnmp.GoSNMP{}
	snmp.Context = context.Background()
	snmp.Community = community
	snmp.Version = gosnmp.Version2c
	snmp.Target = target
	snmp.Port = 161
	snmp.Transport = "udp"
	snmp.Timeout = 30 * time.Second
	snmp.MaxRepetitions = 50
	err := snmp.Connect()
	if err != nil {
		return nil, err
	}

	// Get amount of devices
	devicesResp, err := snmp.Get([]string{"1.3.6.1.4.1.2606.7.4.1.1.2.0"})
	if err != nil {
		return nil, err
	}
	devs := devicesResp.Variables[0].Value.(int)

	// Get amount of variables
	varsResp, err := snmp.Get([]string{"1.3.6.1.4.1.2606.7.4.2.1.1.0"})
	if err != nil {
		return nil, err
	}
	vars := varsResp.Variables[0].Value.(int)

	// Iterate once over all possible devices and get title
	var oids []string

	for x := 1; x <= devs; x++ {
		oids = append(oids, fmt.Sprintf("1.3.6.1.4.1.2606.7.4.1.2.1.3.%d", x))
	}
	devices, err := loadDeviceNames(snmp, oids)
	if err != nil {
		return nil, err
	}

	// Iterate over possible device-variable-combinations
	oids = []string{}
	for x := 1; x <= devs; x++ {
		for y := 1; y <= vars; y++ {
			oids = append(oids, fmt.Sprintf("1.3.6.1.4.1.2606.7.4.2.2.1.3.%d.%d", x, y))
		}
	}
	variables, err := loadVariables(snmp, oids, devices)
	if err != nil {
		return nil, err
	}

	for i, item := range variables {
		packet, err := snmp.Get([]string{fmt.Sprintf("1.3.6.1.4.1.2606.7.4.2.2.1.4.%d.%d", item.Device, item.Variable)})
		if err != nil {
			return nil, err
		}
		variables[i].Type = packet.Variables[0].Value.(int)
	}

	rittalDevices := make(map[string]RittalDevice)
	for _, item := range variables {
		if _, ok := rittalDevices[item.DeviceName]; !ok {
			rittalDevices[item.DeviceName] = RittalDevice{Variables: make(map[string]*QueryObject)}
		}
		nameSegments := strings.Split(item.Name, ".")
		name := strings.Join(nameSegments[:len(nameSegments)-1], ".")
		if _, ok := rittalDevices[item.DeviceName].Variables[name]; !ok {
			rittalDevices[item.DeviceName].Variables[name] = &QueryObject{}
		}
		switch item.Type {
		case 1:
			// Description
			valueOid := fmt.Sprintf("1.3.6.1.4.1.2606.7.4.2.2.1.10.%d.%d", item.Device, item.Variable)
			packet, err := snmp.Get([]string{valueOid})
			if err != nil {
				return nil, err
			}
			desc := packet.Variables[0].Value.([]byte)
			x := rittalDevices[item.DeviceName].Variables[name]
			x.Description = string(desc)
			x.RawTitle = item.Name
		case 2:
			// Value
			valueOid := fmt.Sprintf("1.3.6.1.4.1.2606.7.4.2.2.1.10.%d.%d", item.Device, item.Variable)
			rittalDevices[item.DeviceName].Variables[name].ValueOids = append(rittalDevices[item.DeviceName].Variables[name].ValueOids, SubObject{Name: nameSegments[len(nameSegments)-1:][0], Oid: valueOid})
		case 7:
			// Status
			valueOid := fmt.Sprintf("1.3.6.1.4.1.2606.7.4.2.2.1.11.%d.%d", item.Device, item.Variable)
			rittalDevices[item.DeviceName].Variables[name].StatusOids = append(rittalDevices[item.DeviceName].Variables[name].StatusOids, SubObject{Name: nameSegments[len(nameSegments)-1:][0], Oid: valueOid})
		}
	}

	return rittalDevices, nil
}

func loadSnmpData(snmp gosnmp.GoSNMP, oids []string) ([]gosnmp.SnmpPDU, error) {
	maxOids := int(snmp.MaxRepetitions)
	var variables []gosnmp.SnmpPDU
	// Max Repetition can be 0, maxOids cannot. SNMPv1 can only report one OID error per call.
	if maxOids == 0 || snmp.Version == gosnmp.Version1 {
		maxOids = 1
	}
	for len(oids) > 0 {

		nOids := len(oids)
		if nOids > maxOids {
			nOids = maxOids
		}

		packet, err := snmp.Get(oids[:nOids])
		if err != nil {
			return nil, err
		}
		// SNMPv1 will return packet error for unsupported OIDs.
		if packet.Error == gosnmp.NoSuchName && snmp.Version == gosnmp.Version1 {
			continue
		}
		// Response received with errors.
		// TODO: "stringify" gosnmp errors instead of showing error code.
		if packet.Error != gosnmp.NoError {
			continue
		}
		for _, v := range packet.Variables {
			if v.Type == gosnmp.NoSuchObject || v.Type == gosnmp.NoSuchInstance {
				continue
			}
			variables = append(variables, v)
		}
		oids = oids[nOids:]
	}
	return variables, nil
}

func loadVariables(snmp gosnmp.GoSNMP, oids []string, devices []string) ([]RittalVariable, error) {
	var variables []RittalVariable
	data, err := loadSnmpData(snmp, oids)
	if err != nil {
		return nil, err
	}
	for _, v := range data {
		bytes := v.Value.([]byte)
		d := strings.Split(v.Name, ".")
		i, _ := strconv.Atoi(d[len(d)-2 : len(d)-1][0])
		j, _ := strconv.Atoi(d[len(d)-1:][0])
		variables = append(variables, RittalVariable{Name: string(bytes), Device: i, DeviceName: devices[i-1], Variable: j})
	}
	return variables, nil
}

func loadDeviceNames(snmp gosnmp.GoSNMP, oids []string) ([]string, error) {
	var devices []string

	data, err := loadSnmpData(snmp, oids)
	if err != nil {
		return nil, err
	}
	for _, v := range data {
		bytes := v.Value.([]byte)
		devices = append(devices, string(bytes))
	}
	return devices, nil
}
