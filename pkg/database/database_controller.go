package database

import (
	"context"
	"errors"
	"strings"

	"github.com/go-logr/logr"
	databasev1alpha1 "github.com/pluralsh/database-interface-api/apis/database/v1alpha1"
	databasespec "github.com/pluralsh/database-interface-api/spec"
	"github.com/pluralsh/database-interface-controller/pkg/kubernetes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	DatabaseAccessFinalizer  = "pluralsh.database-interface-controller/databaseaccess-database-protection"
	DatabaseFinalizer        = "pluralsh.database-interface-controller/database-protection"
	DatabaseRequestFinalizer = "pluralsh.database-interface-controller/databaserequest-protection"
)

// Reconciler reconciles a DatabaseRequest object
type Reconciler struct {
	client.Client
	Log logr.Logger

	DriverName        string
	ProvisionerClient databasespec.ProvisionerClient
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("Database", req.NamespacedName)

	var database databasev1alpha1.Database
	if err := r.Get(ctx, req.NamespacedName, &database); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !database.GetDeletionTimestamp().IsZero() {
		if controllerutil.ContainsFinalizer(&database, DatabaseAccessFinalizer) {
			databaseReqNs := database.Spec.DatabaseRequest.Namespace
			databaseReqName := database.Spec.DatabaseRequest.Name

			var databaseAccessList databasev1alpha1.DatabaseAccessList
			if err := r.List(ctx, &databaseAccessList, client.InNamespace(databaseReqNs)); err != nil {
				log.Error(err, "Failed to get DatabaseAccessList")
				return ctrl.Result{}, err
			}
			for _, databaseAccess := range databaseAccessList.Items {
				if strings.EqualFold(databaseAccess.Spec.DatabaseRequestName, databaseReqName) {
					if err := r.Delete(ctx, &databasev1alpha1.DatabaseAccess{
						ObjectMeta: metav1.ObjectMeta{Name: databaseAccess.Name, Namespace: databaseReqNs},
					}); err != nil {
						log.Error(err, "Failed to delete DatabaseAccess")
						return ctrl.Result{}, err
					}
				}
			}
			if err := kubernetes.TryRemoveFinalizer(ctx, r.Client, &database, DatabaseAccessFinalizer); err != nil {
				return ctrl.Result{}, err
			}
		}
		if controllerutil.ContainsFinalizer(&database, DatabaseFinalizer) {
			if err := r.deleteDatabaseOp(ctx, &database); err != nil {
				log.Error(err, "Failed to delete Database")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !strings.EqualFold(database.Spec.DriverName, r.DriverName) {
		return ctrl.Result{}, nil
	}

	if database.Status.Ready {
		return ctrl.Result{}, nil
	}

	databaseReady := false
	var databaseID string

	if database.Spec.ExistingDatabaseID == "" {
		req := &databasespec.DriverCreateDatabaseRequest{
			Parameters: database.Spec.Parameters,
			Name:       database.ObjectMeta.Name,
		}
		rsp, err := r.ProvisionerClient.DriverCreateDatabase(ctx, req)
		if err != nil {
			if status.Code(err) != codes.AlreadyExists {
				log.Error(err, "Driver failed to create database")
				return ctrl.Result{}, err
			}
		}
		if rsp == nil {
			err = errors.New("DriverCreateDatabase returned a nil response")
			log.Error(err, "Internal Error from driver")
			return ctrl.Result{}, err
		}

		if rsp.DatabaseId != "" {
			databaseID = rsp.DatabaseId
			databaseReady = true
		} else {
			log.Error(err, "DriverCreateDatabase returned an empty databaseID")
			err = errors.New("DriverCreateDatabase returned an empty databaseID")
			return ctrl.Result{}, err
		}
		// Now we update the DatabaseReady status of DatabaseRequest
		if database.Spec.DatabaseRequest != nil {
			ref := database.Spec.DatabaseRequest

			var databaseReq databasev1alpha1.DatabaseRequest
			if err := r.Get(ctx, client.ObjectKey{
				Namespace: ref.Namespace,
				Name:      ref.Name,
			}, &databaseReq); err != nil {
				log.Error(err, "Failed to get database request")
				return ctrl.Result{}, err
			}
			databaseReqCopy := databaseReq.DeepCopy()
			databaseReqCopy.Status.Ready = true
			databaseReqCopy.Status.DatabaseName = database.Name
			if err := r.Status().Update(ctx, databaseReqCopy); err != nil {
				log.Error(err, "Failed to update DatabaseRequest status")
				return ctrl.Result{}, err
			}
			log.Info("Successfully updated status of DatabaseRequest")
		}
	} else {
		databaseReady = true
		databaseID = database.Spec.ExistingDatabaseID
	}

	if err := kubernetes.TryAddFinalizer(ctx, r.Client, &database, DatabaseFinalizer); err != nil {
		log.Error(err, "Can't update finalizer")
		return ctrl.Result{}, err
	}

	database.Status.Ready = databaseReady
	database.Status.DatabaseID = databaseID
	if err := r.Status().Update(ctx, &database); err != nil {
		log.Error(err, "Can't update database")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) deleteDatabaseOp(ctx context.Context, database *databasev1alpha1.Database) error {
	if !strings.EqualFold(database.Spec.DriverName, r.DriverName) {
		return nil
	}
	if database.Spec.DeletionPolicy == databasev1alpha1.DeletionPolicyDelete {
		req := &databasespec.DriverDeleteDatabaseRequest{
			DatabaseId: database.Status.DatabaseID,
		}
		if _, err := r.ProvisionerClient.DriverDeleteDatabase(ctx, req); err != nil {
			if status.Code(err) != codes.NotFound {
				return err
			}
		}
	}

	kubernetes.TryRemoveFinalizer(ctx, r.Client, database, DatabaseFinalizer)

	if database.Spec.DatabaseRequest != nil {
		ref := database.Spec.DatabaseRequest
		databaseRequest := &databasev1alpha1.DatabaseRequest{}
		if err := r.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, databaseRequest); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			return nil
		}
		return kubernetes.TryRemoveFinalizer(ctx, r.Client, databaseRequest, DatabaseRequestFinalizer)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.Database{}).
		Complete(r)
}
