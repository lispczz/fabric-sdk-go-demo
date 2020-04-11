/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package test

import (
	"github.com/hyperledger/fabric-sdk-go/pkg/core/config"
	"testing"
)

func TestE2E(t *testing.T) {
	Run(t, config.FromFile("config.yaml"))
}
