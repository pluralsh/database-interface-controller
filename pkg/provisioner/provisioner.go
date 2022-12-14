/* Copyright 2021 The Kubernetes Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package provisioner

import (
	"context"
	"net/url"
	"time"

	"github.com/pkg/errors"
	databasespec "github.com/pluralsh/database-interface-api/spec"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"k8s.io/klog/v2"
)

const (
	maxGrpcBackoff  = 5 * 30 * time.Second
	grpcDialTimeout = 30 * time.Second
)

func NewDefaultProvisionerClient(ctx context.Context, address string, debug bool) (*ProvisionerClient, error) {
	backoffConfiguration := backoff.DefaultConfig
	backoffConfiguration.MaxDelay = maxGrpcBackoff

	dialOpts := []grpc.DialOption{
		grpc.WithInsecure(), // strictly restricting to local Unix domain socket
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoffConfiguration,
			MinConnectTimeout: grpcDialTimeout,
		}),
		grpc.WithBlock(), // block until connection succeeds
	}

	interceptors := []grpc.UnaryClientInterceptor{}
	if debug {
		interceptors = append(interceptors, apiLogger)
	}
	return NewProvisionerClient(ctx, address, dialOpts, interceptors)
}

// NewCOSIProvisionerClient creates a new GRPCClient that only supports unix domain sockets
func NewProvisionerClient(ctx context.Context, address string, dialOpts []grpc.DialOption, interceptors []grpc.UnaryClientInterceptor) (*ProvisionerClient, error) {
	addr, err := url.Parse(address)
	if err != nil {
		return nil, err
	}

	if addr.Scheme != "unix" {
		err := errors.New("Address must be a unix domain socket")
		klog.ErrorS(err, "Unsupported scheme", "expected", "unix", "found", addr.Scheme)
		return nil, errors.Wrap(err, "Invalid argument")
	}

	for _, interceptor := range interceptors {
		dialOpts = append(dialOpts, grpc.WithChainUnaryInterceptor(interceptor))
	}

	ctx, cancel := context.WithTimeout(ctx, maxGrpcBackoff)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address, dialOpts...)
	if err != nil {
		klog.ErrorS(err, "Connection failed", "address", address)
		return nil, err
	}
	return &ProvisionerClient{
		address:           address,
		conn:              conn,
		identityClient:    databasespec.NewIdentityClient(conn),
		provisionerClient: databasespec.NewProvisionerClient(conn),
	}, nil
}

func NewDefaultProvisionerServer(address string,
	identityServer databasespec.IdentityServer,
	provisionerServer databasespec.ProvisionerServer) (*ProvisionerServer, error) {

	return NewProvisionerServer(address, identityServer, provisionerServer, []grpc.ServerOption{})
}

func NewProvisionerServer(address string,
	identityServer databasespec.IdentityServer,
	provisionerServer databasespec.ProvisionerServer,
	listenOpts []grpc.ServerOption) (*ProvisionerServer, error) {

	if identityServer == nil {
		err := errors.New("Identity server cannot be nil")
		klog.ErrorS(err, "Invalid argument")
		return nil, err
	}
	if provisionerServer == nil {
		err := errors.New("Provisioner server cannot be nil")
		klog.ErrorS(err, "Invalid argument")
		return nil, err
	}

	return &ProvisionerServer{
		address:           address,
		identityServer:    identityServer,
		provisionerServer: provisionerServer,
		listenOpts:        listenOpts,
	}, nil
}
