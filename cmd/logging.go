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
	"flag"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// newLogOptions keeps the operator in production logging mode unless an
// operator explicitly opts into development logging with --zap-devel.
func newLogOptions(fs *flag.FlagSet) *zap.Options {
	opts := &zap.Options{Development: false}
	opts.BindFlags(fs)
	return opts
}

// applyLogLevelEnv applies LOG_LEVEL when the equivalent controller-runtime
// flag was not supplied. Applying the environment value after flag parsing
// keeps an explicit --zap-log-level argument authoritative.
func applyLogLevelEnv(fs *flag.FlagSet, value string) error {
	if value == "" || flagWasSet(fs, "zap-log-level") {
		return nil
	}

	if err := fs.Set("zap-log-level", value); err != nil {
		return fmt.Errorf("invalid LOG_LEVEL %q: %w", value, err)
	}
	return nil
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	wasSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			wasSet = true
		}
	})
	return wasSet
}
