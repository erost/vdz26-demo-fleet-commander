package main

import (
	sdk "github.com/crossplane/function-sdk-go"
	"github.com/crossplane/function-sdk-go/logging"
)

func main() {
	if err := sdk.Serve(&Function{log: logging.NewNopLogger()}, sdk.MTLSCertificates("/tls/server")); err != nil {
		panic(err)
	}
}
