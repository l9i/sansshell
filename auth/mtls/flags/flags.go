/* Copyright (c) 2019 Snowflake Inc. All rights reserved.

   Licensed under the Apache License, Version 2.0 (the
   "License"); you may not use this file except in compliance
   with the License.  You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing,
   software distributed under the License is distributed on an
   "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
   KIND, either express or implied.  See the License for the
   specific language governing permissions and limitations
   under the License.
*/

// Package flags provides flag support for loading client/server certs and CA root of trust.
package flags

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"log"
	"os"
	"path"

	"github.com/Snowflake-Labs/sansshell/auth/mtls"
)

const (
	loaderName = "flags"

	defaultClientCertPath = ".sansshell/client.pem"
	defaultClientKeyPath  = ".sansshell/client.key"
	defaultServerCertPath = ".sansshell/leaf.pem"
	defaultServerKeyPath  = ".sansshell/leaf.key"
	defaultRootCAPath     = ".sansshell/root.pem"
)

var (
	clientCertFile, clientKeyFile string
	serverCertFile, serverKeyFile string
	rootCAFile                    string
)

// Name returns the loader to use to set mtls params via flags.
func Name() string { return loaderName }

// flagLoader implements mtls.CredentialsLoader by reading files specified
// by command-line flags.
type flagLoader struct{}

func (flagLoader) LoadClientCA(context.Context) (*x509.CertPool, error) {
	return mtls.LoadRootOfTrust(rootCAFile)
}

func (flagLoader) LoadRootCA(context.Context) (*x509.CertPool, error) {
	return mtls.LoadRootOfTrust(rootCAFile)
}

func (flagLoader) LoadClientCertificate(context.Context) (tls.Certificate, error) {
	return tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
}

func (flagLoader) LoadServerCertificate(context.Context) (tls.Certificate, error) {
	return tls.LoadX509KeyPair(serverCertFile, serverKeyFile)
}

func (flagLoader) CertsRefreshed() bool {
	return false
}

func init() {
	cd, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	clientCertFile = path.Join(cd, defaultClientCertPath)
	clientKeyFile = path.Join(cd, defaultClientKeyPath)
	serverCertFile = path.Join(cd, defaultServerCertPath)
	serverKeyFile = path.Join(cd, defaultServerKeyPath)
	rootCAFile = path.Join(cd, defaultRootCAPath)

	flag.StringVar(&clientCertFile, "client-cert", clientCertFile, "Path to this client's x509 cert, PEM format")
	flag.StringVar(&clientKeyFile, "client-key", clientKeyFile, "Path to this client's key")
	flag.StringVar(&serverCertFile, "server-cert", serverCertFile, "Path to an x509 server cert, PEM format")
	flag.StringVar(&serverKeyFile, "server-key", serverKeyFile, "Path to the server's TLS key")
	flag.StringVar(&rootCAFile, "root-ca", rootCAFile, "The root of trust for remote identities, PEM format")

	if err := mtls.Register(loaderName, flagLoader{}); err != nil {
		panic(err)
	}
}
