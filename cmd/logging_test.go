/*
Copyright 2024 RajSingh.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestLogOptionsDefaultToProduction(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	opts := newLogOptions(fs)

	if opts.Development {
		t.Fatal("default logging mode must be production")
	}

	output := captureLogOutput(opts)
	if !strings.Contains(output, "info message") {
		t.Fatalf("default logger omitted info message: %s", output)
	}
	if strings.Contains(output, "verbose message") {
		t.Fatalf("default logger emitted verbose message: %s", output)
	}
	if !strings.HasPrefix(strings.TrimSpace(output), "{") {
		t.Fatalf("default logger should use production JSON encoding: %s", output)
	}
}

func TestLogLevelEnvironmentVariable(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		wantInfo    bool
		wantVerbose bool
		wantError   bool
	}{
		{name: "info", value: "info", wantInfo: true, wantError: true},
		{name: "error", value: "error", wantError: true},
		{name: "debug", value: "debug", wantInfo: true, wantVerbose: true, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			opts := newLogOptions(fs)
			if err := applyLogLevelEnv(fs, tt.value); err != nil {
				t.Fatalf("applyLogLevelEnv() error = %v", err)
			}

			output := captureLogOutput(opts)
			assertLogMessage(t, output, "info message", tt.wantInfo)
			assertLogMessage(t, output, "verbose message", tt.wantVerbose)
			assertLogMessage(t, output, "error message", tt.wantError)
		})
	}
}

func TestLogLevelFlagTakesPrecedenceOverEnvironment(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	opts := newLogOptions(fs)
	if err := fs.Parse([]string{"--zap-log-level=debug"}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := applyLogLevelEnv(fs, "error"); err != nil {
		t.Fatalf("applyLogLevelEnv() error = %v", err)
	}

	output := captureLogOutput(opts)
	assertLogMessage(t, output, "verbose message", true)
}

func TestLogLevelEnvironmentVariableRejectsInvalidValue(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	newLogOptions(fs)

	err := applyLogLevelEnv(fs, "nope")
	if err == nil {
		t.Fatal("applyLogLevelEnv() accepted an invalid log level")
	}
	if !strings.Contains(err.Error(), "invalid LOG_LEVEL") {
		t.Fatalf("error = %v, want LOG_LEVEL context", err)
	}
}

func captureLogOutput(opts *zap.Options) string {
	var output bytes.Buffer
	logger := zap.New(zap.UseFlagOptions(opts), zap.WriteTo(&output))
	logger.Info("info message")
	logger.V(1).Info("verbose message")
	logger.Error(errors.New("test error"), "error message")
	return output.String()
}

func assertLogMessage(t *testing.T, output, message string, want bool) {
	t.Helper()
	got := strings.Contains(output, message)
	if got != want {
		t.Fatalf("log output contains %q = %t, want %t: %s", message, got, want, output)
	}
}
