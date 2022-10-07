package main

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/util/rand"

	databasespec "github.com/pluralsh/database-interface-api/spec"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

func NewDriver(provisioner string) (*IdentityServer, *ProvisionerServer) {
	return &IdentityServer{
			provisioner: provisioner,
		}, &ProvisionerServer{
			provisioner: provisioner,
			database:    map[string]string{},
		}
}

type ProvisionerServer struct {
	provisioner string
	database    map[string]string
}

func (ps *ProvisionerServer) DriverCreateDatabase(_ context.Context, req *databasespec.DriverCreateDatabaseRequest) (*databasespec.DriverCreateDatabaseResponse, error) {
	databaseName := req.GetName()
	klog.V(3).InfoS("Create Database", "name", databaseName)

	if ps.database[databaseName] != "" {
		return &databasespec.DriverCreateDatabaseResponse{}, status.Error(codes.AlreadyExists, "Database already exists")
	}
	dbID := MakeDatabaseID()
	ps.database[databaseName] = dbID

	return &databasespec.DriverCreateDatabaseResponse{
		DatabaseId: dbID,
	}, nil
}

func (ps *ProvisionerServer) DriverDeleteDatabase(_ context.Context, req *databasespec.DriverDeleteDatabaseRequest) (*databasespec.DriverDeleteDatabaseResponse, error) {
	for name, id := range ps.database {
		if req.DatabaseId == id {
			delete(ps.database, name)
			return &databasespec.DriverDeleteDatabaseResponse{}, nil
		}
	}
	return &databasespec.DriverDeleteDatabaseResponse{}, status.Error(codes.NotFound, "Database not found")
}

// This call grants access to an account. The account_name in the request shall be used as a unique identifier to create credentials.
// The account_id returned in the response will be used as the unique identifier for deleting this access when calling DriverRevokeDatabaseAccess.
func (ps *ProvisionerServer) DriverGrantDatabaseAccess(context.Context, *databasespec.DriverGrantDatabaseAccessRequest) (*databasespec.DriverGrantDatabaseAccessResponse, error) {
	resp := &databasespec.DriverGrantDatabaseAccessResponse{
		AccountId:   "abc",
		Credentials: map[string]*databasespec.CredentialDetails{},
	}
	resp.Credentials["cred"] = &databasespec.CredentialDetails{Secrets: map[string]string{"a": "b"}}

	return resp, nil
}

// This call revokes all access to a particular database from a principal.
func (ps *ProvisionerServer) DriverRevokeDatabaseAccess(context.Context, *databasespec.DriverRevokeDatabaseAccessRequest) (*databasespec.DriverRevokeDatabaseAccessResponse, error) {
	return &databasespec.DriverRevokeDatabaseAccessResponse{}, nil
}

type IdentityServer struct {
	provisioner string
}

func (id *IdentityServer) DriverGetInfo(context.Context, *databasespec.DriverGetInfoRequest) (*databasespec.DriverGetInfoResponse, error) {
	if id.provisioner == "" {
		klog.ErrorS(errors.New("provisioner name cannot be empty"), "Invalid argument")
		return nil, status.Error(codes.InvalidArgument, "ProvisionerName is empty")
	}

	return &databasespec.DriverGetInfoResponse{
		Name: id.provisioner,
	}, nil
}

func MakeDatabaseID() string {
	alpha := "abcdefghijklmnopqrstuvwxyz"
	r := rand.Intn(len(alpha))
	return fmt.Sprintf("%c%s", alpha[r], rand.String(9))
}
