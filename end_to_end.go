/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package test

import (
	"github.com/hyperledger/fabric-sdk-go/pkg/client/channel"
	mspclient "github.com/hyperledger/fabric-sdk-go/pkg/client/msp"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/resmgmt"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/errors/retry"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/core"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/fab"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/msp"
	packager "github.com/hyperledger/fabric-sdk-go/pkg/fab/ccpackager/gopackager"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk"
	"github.com/hyperledger/fabric-sdk-go/third_party/github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
	"time"
)

const (
	channelID      = "mychannel"
	orgName        = "Org1"
	orgAdmin       = "Admin"
	ordererOrgName = "orderer.example.com"
	peerTarget     = "peer0.org1.example.com"
)

var (
	ccID = "example_cc_e2e"
)

// Run enables testing an end-to-end scenario against the supplied SDK options
func Run(t *testing.T, configOpt core.ConfigProvider, sdkOpts ...fabsdk.Option) {
	setupAndRun(t, true, configOpt, e2eTest, sdkOpts...)
}

// RunWithoutSetup will execute the same way as Run but without creating a new channel and registering a new CC
func RunWithoutSetup(t *testing.T, configOpt core.ConfigProvider, sdkOpts ...fabsdk.Option) {
	setupAndRun(t, false, configOpt, e2eTest, sdkOpts...)
}

type testSDKFunc func(t *testing.T, sdk *fabsdk.FabricSDK)

// setupAndRun enables testing an end-to-end scenario against the supplied SDK options
// the createChannel flag will be used to either create a channel and the example CC or not(ie run the tests with existing ch and CC)
func setupAndRun(t *testing.T, createChannel bool, configOpt core.ConfigProvider, test testSDKFunc, sdkOpts ...fabsdk.Option) {
	sdk, err := fabsdk.New(configOpt, sdkOpts...)
	if err != nil {
		t.Fatalf("Failed to create new SDK: %s", err)
	}
	defer sdk.Close()

	if createChannel {
		createChannelAndCC(t, sdk)
	}

	test(t, sdk)
}

func e2eTest(t *testing.T, sdk *fabsdk.FabricSDK) {
	//prepare channel client context using client context
	clientChannelContext := sdk.ChannelContext(channelID, fabsdk.WithUser("User1"), fabsdk.WithOrg(orgName))
	// Channel client is used to query and execute transactions (Org1 is default org)
	client, err := channel.New(clientChannelContext)
	if err != nil {
		t.Fatalf("Failed to create new channel client: %s", err)
	}

	existingValue := queryCC(t, client)
	ccEvent := moveFunds(t, client)

	// Verify move funds transaction result on the same peer where the event came from.
	verifyFundsIsMoved(t, client, existingValue, ccEvent)
}

func createChannelAndCC(t *testing.T, sdk *fabsdk.FabricSDK) {
	//clientContext allows creation of transactions using the supplied identity as the credential.
	clientContext := sdk.Context(fabsdk.WithUser(orgAdmin), fabsdk.WithOrg(orgName))

	// Resource management client is responsible for managing channels (create/update channel)
	// Supply user that has privileges to create channel (in this case orderer admin)
	resMgmtClient, err := resmgmt.New(clientContext)
	if err != nil {
		t.Fatalf("Failed to create channel management client: %s", err)
	}

	existed := false
	allChannels, err := resMgmtClient.QueryChannels(resmgmt.WithTargetEndpoints(peerTarget))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range allChannels.Channels {
		if item.ChannelId == channelID {
			existed = true
			break
		}
	}
	if !existed {
		// Create channel
		createChannel(t, sdk, resMgmtClient)

		//prepare context
		adminContext := sdk.Context(fabsdk.WithUser(orgAdmin), fabsdk.WithOrg(orgName))

		// Org resource management client
		orgResMgmt, err := resmgmt.New(adminContext)
		if err != nil {
			t.Fatalf("Failed to create new resource management client: %s", err)
		}

		// Org peers join channel
		if err = orgResMgmt.JoinChannel(channelID, resmgmt.WithRetry(retry.DefaultResMgmtOpts), resmgmt.WithOrdererEndpoint("orderer.example.com")); err != nil {
			t.Fatalf("Org peers failed to JoinChannel: %s", err)
		}
	}

	// Create chaincode package for example cc
	createCC(t, resMgmtClient)
}

func moveFunds(t *testing.T, client *channel.Client) *fab.CCEvent {

	eventID := "test([a-zA-Z]+)"

	// Register chaincode event (pass in channel which receives event details when the event is complete)
	reg, notifier, err := client.RegisterChaincodeEvent(ccID, eventID)
	if err != nil {
		t.Fatalf("Failed to register cc event: %s", err)
	}
	defer client.UnregisterChaincodeEvent(reg)

	// Move funds
	executeCC(t, client)

	var ccEvent *fab.CCEvent
	select {
	case ccEvent = <-notifier:
		t.Logf("Received CC event: %#v\n", ccEvent)
	case <-time.After(time.Second * 20):
		t.Fatalf("Did NOT receive CC event for eventId(%s)\n", eventID)
	}

	return ccEvent
}

func verifyFundsIsMoved(t *testing.T, client *channel.Client, value []byte, ccEvent *fab.CCEvent) {

	newValue := queryCC(t, client, ccEvent.SourceURL)
	valueInt, err := strconv.Atoi(string(value))
	if err != nil {
		t.Fatal(err.Error())
	}
	valueAfterInvokeInt, err := strconv.Atoi(string(newValue))
	if err != nil {
		t.Fatal(err.Error())
	}
	if valueInt+1 != valueAfterInvokeInt {
		t.Fatalf("Execute failed. Before: %s, after: %s", value, newValue)
	}
}

func executeCC(t *testing.T, client *channel.Client) {
	_, err := client.Execute(channel.Request{ChaincodeID: ccID, Fcn: "invoke", Args: [][]byte{[]byte("move"), []byte("a"), []byte("b"), []byte("1")}},
		channel.WithRetry(retry.DefaultChannelOpts))
	if err != nil {
		t.Fatalf("Failed to move funds: %s", err)
	}
}

func queryCC(t *testing.T, client *channel.Client, targetEndpoints ...string) []byte {
	response, err := client.Query(channel.Request{ChaincodeID: ccID, Fcn: "invoke", Args: [][]byte{[]byte("query"), []byte("b")}},
		channel.WithRetry(retry.DefaultChannelOpts),
		channel.WithTargetEndpoints(targetEndpoints...),
	)
	if err != nil {
		t.Fatalf("Failed to query funds: %s", err)
	}
	return response.Payload
}

func createCC(t *testing.T, orgResMgmt *resmgmt.Client) {
	ccPkg, err := packager.NewCCPackage("github.com/example_cc", "chaincode")
	if err != nil {
		t.Fatal(err)
	}
	// Install example cc to org peers
	installCCReq := resmgmt.InstallCCRequest{Name: ccID, Path: "github.com/example_cc", Version: "0", Package: ccPkg}
	_, err = orgResMgmt.InstallCC(installCCReq, resmgmt.WithRetry(retry.DefaultResMgmtOpts))
	if err != nil {
		t.Fatal(err)
	}
	// Set up chaincode policy
	ccPolicy := cauthdsl.SignedByAnyMember([]string{"Org1MSP"})
	// Org resource manager will instantiate 'example_cc' on channel
	resp, err := orgResMgmt.InstantiateCC(
		channelID,
		resmgmt.InstantiateCCRequest{Name: ccID, Path: "github.com/example_cc", Version: "0", Args: [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, Policy: ccPolicy},
		resmgmt.WithRetry(retry.DefaultResMgmtOpts),
	)
	require.Nil(t, err, "error should be nil")
	require.NotEmpty(t, resp, "transaction response should be populated")
}

func createChannel(t *testing.T, sdk *fabsdk.FabricSDK, resMgmtClient *resmgmt.Client) {
	mspClient, err := mspclient.New(sdk.Context(), mspclient.WithOrg(orgName))
	if err != nil {
		t.Fatal(err)
	}
	adminIdentity, err := mspClient.GetSigningIdentity(orgAdmin)
	if err != nil {
		t.Fatal(err)
	}
	req := resmgmt.SaveChannelRequest{ChannelID: channelID,
		ChannelConfigPath: "fixtures/artifacts/" + (channelID + ".tx"),
		SigningIdentities: []msp.SigningIdentity{adminIdentity}}
	txID, err := resMgmtClient.SaveChannel(req, resmgmt.WithRetry(retry.DefaultResMgmtOpts), resmgmt.WithOrdererEndpoint("orderer.example.com"))
	require.Nil(t, err, "error should be nil")
	require.NotEmpty(t, txID, "transaction ID should be populated")
}
