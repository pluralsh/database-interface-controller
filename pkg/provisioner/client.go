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

	databasespec "github.com/pluralsh/database-interface-api/spec"
	"google.golang.org/grpc"
)

var (
	_ databasespec.IdentityClient    = &ProvisionerClient{}
	_ databasespec.ProvisionerClient = &ProvisionerClient{}
)

type ProvisionerClient struct {
	address           string
	conn              *grpc.ClientConn
	identityClient    databasespec.IdentityClient
	provisionerClient databasespec.ProvisionerClient
}

func (c *ProvisionerClient) DriverGetInfo(ctx context.Context,
	in *databasespec.DriverGetInfoRequest,
	opts ...grpc.CallOption) (*databasespec.DriverGetInfoResponse, error) {

	return c.identityClient.DriverGetInfo(ctx, in, opts...)
}

func (c *ProvisionerClient) DriverCreateDatabase(ctx context.Context,
	in *databasespec.DriverCreateDatabaseRequest,
	opts ...grpc.CallOption) (*databasespec.DriverCreateDatabaseResponse, error) {

	return c.provisionerClient.DriverCreateDatabase(ctx, in, opts...)
}

func (c *ProvisionerClient) DriverDeleteDatabase(ctx context.Context,
	in *databasespec.DriverDeleteDatabaseRequest,
	opts ...grpc.CallOption) (*databasespec.DriverDeleteDatabaseResponse, error) {

	return c.provisionerClient.DriverDeleteDatabase(ctx, in, opts...)
}

func (c *ProvisionerClient) DriverGrantDatabaseAccess(ctx context.Context,
	in *databasespec.DriverGrantDatabaseAccessRequest,
	opts ...grpc.CallOption) (*databasespec.DriverGrantDatabaseAccessResponse, error) {

	return c.provisionerClient.DriverGrantDatabaseAccess(ctx, in, opts...)
}

func (c *ProvisionerClient) DriverRevokeDatabaseAccess(ctx context.Context,
	in *databasespec.DriverRevokeDatabaseAccessRequest,
	opts ...grpc.CallOption) (*databasespec.DriverRevokeDatabaseAccessResponse, error) {

	return c.provisionerClient.DriverRevokeDatabaseAccess(ctx, in, opts...)
}
