// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-2020 Datadog, Inc.

package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
)
import "github.com/DataDog/datadog-agent/pkg/traceinit"

var (
	dsdCaptureDuration   time.Duration
	dsdCaptureFilePath   string
	dsdCaptureCompressed bool
)

const (
	defaultCaptureDuration = time.Duration(1) * time.Minute
)

func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\agent\app\dogstatsd_capture.go 39`)
	AgentCmd.AddCommand(dogstatsdCaptureCmd)
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\agent\app\dogstatsd_capture.go 40`)
	dogstatsdCaptureCmd.Flags().DurationVarP(&dsdCaptureDuration, "duration", "d", defaultCaptureDuration, "Duration traffic capture should span.")
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\agent\app\dogstatsd_capture.go 41`)
	dogstatsdCaptureCmd.Flags().StringVarP(&dsdCaptureFilePath, "path", "p", "", "Directory path to write the capture to.")
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\agent\app\dogstatsd_capture.go 42`)
	dogstatsdCaptureCmd.Flags().BoolVarP(&dsdCaptureCompressed, "compressed", "z", true, "Should capture be zstd compressed.")
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\agent\app\dogstatsd_capture.go 43`)

	// shut up grpc client!
	grpclog.SetLoggerV2(grpclog.NewLoggerV2(ioutil.Discard, ioutil.Discard, ioutil.Discard))
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\agent\app\dogstatsd_capture.go 46`)
}

var dogstatsdCaptureCmd = &cobra.Command{
	Use:   "dogstatsd-capture",
	Short: "Start a dogstatsd UDS traffic capture",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfigWithoutSecrets(confFilePath, "")
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		err = config.SetupLogger(loggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		return dogstatsdCapture()
	},
}

func dogstatsdCapture() error {
	fmt.Printf("Starting a dogstatsd traffic capture session...\n\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	token, err := security.FetchAuthToken()
	if err != nil {
		return fmt.Errorf("unable to fetch authentication token: %w", err)
	}

	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	// NOTE: we're using InsecureSkipVerify because the gRPC server only
	// persists its TLS certs in memory, and we currently have no
	// infrastructure to make them available to clients. This is NOT
	// equivalent to grpc.WithInsecure(), since that assumes a non-TLS
	// connection.
	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	conn, err := grpc.DialContext(
		ctx,
		fmt.Sprintf(":%v", config.Datadog.GetInt("cmd_port")),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return err
	}

	cli := pb.NewAgentSecureClient(conn)

	resp, err := cli.DogstatsdCaptureTrigger(ctx, &pb.CaptureTriggerRequest{
		Duration:   dsdCaptureDuration.String(),
		Path:       dsdCaptureFilePath,
		Compressed: dsdCaptureCompressed,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Capture started, capture file being written to: %s\n", resp.Path)

	return nil
}
