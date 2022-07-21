// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package config

import (
	"fmt"
	"net"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)
import "github.com/DataDog/datadog-agent/pkg/traceinit"

const (
	// defaultSystemProbeAddress is the default address to be used for connecting to the system probe
	defaultSystemProbeAddress = "localhost:3333"

	// ServiceName is the service name used for the system-probe
	ServiceName = "datadog-system-probe"
)

var (
	defaultConfigDir              = "c:\\programdata\\datadog\\"
	defaultSystemProbeLogFilePath = "c:\\programdata\\datadog\\logs\\system-probe.log"
)

func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\system-probe\config\config_windows.go 32`)
	pd, err := winutil.GetProgramDataDir()
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\system-probe\config\config_windows.go 33`)
	if err == nil {
		defaultConfigDir = pd
		defaultSystemProbeLogFilePath = filepath.Join(pd, "logs", "system-probe.log")
	}
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\system-probe\config\config_windows.go 37`)
}

// ValidateSocketAddress validates that the sysprobe socket config option is of the correct format.
func ValidateSocketAddress(sockAddress string) error {
	if _, _, err := net.SplitHostPort(sockAddress); err != nil {
		return fmt.Errorf("socket address must be of the form 'host:port'")
	}
	return nil
}
