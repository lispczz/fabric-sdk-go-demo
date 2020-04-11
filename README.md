# Introduction

Hyperlegder Fabric Golang SDK is notoriously difficult to configure and use. Here is an end to end demo showing how to install and invoke a chaincode on the new "test-network" of Fabric 2.0 using Golang SDK. 

## How to launch

### Setup Environment
First, install dependencies following [Fabric Docs](https://hyperledger-fabric.readthedocs.io/en/release-2.0/prereqs.html).   
Then, clone the code and `cd` into it:

```bash
git clone ...
cd fabric-sdk-go-demo
```   

Then, install Fabric:

```bash
# run the following cmd in repo root folder
curl -sSL https://raw.githubusercontent.com/hyperledger/fabric/master/scripts/bootstrap.sh | bash -s -- 2.0.1 1.4.6 0.4.18
cd fabric-samples
# Disable fabric chaincode lifecycle which in not support in Fabric Golang SDK yet
git apply ../fabric-samples.patch
cd test-network
# Launch Fabric network
./network.sh up createChannel -s couchdb
```

### Test

```bash
# return to project folder
cd ../..
go test
```

