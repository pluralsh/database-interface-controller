package database

import (
	"context"
	"errors"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"github.com/pluralsh/controller-reconcile-helper/pkg/conditions"
	"github.com/pluralsh/controller-reconcile-helper/pkg/patch"
	crhelperTypes "github.com/pluralsh/controller-reconcile-helper/pkg/types"
	databasev1alpha1 "github.com/pluralsh/database-interface-api/apis/database/v1alpha1"
	databasespec "github.com/pluralsh/database-interface-api/spec"
	"github.com/pluralsh/database-interface-controller/pkg/kubernetes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
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

	database := &databasev1alpha1.Database{}
	if err := r.Get(ctx, req.NamespacedName, database); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	patchHelper, err := patch.NewHelper(database, r.Client)
	if err != nil {
		log.Error(err, "Error getting patchHelper for Database")
		return ctrl.Result{}, err
	}

	if !database.GetDeletionTimestamp().IsZero() {
		if controllerutil.ContainsFinalizer(database, DatabaseAccessFinalizer) {
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
			if err := kubernetes.TryRemoveFinalizer(ctx, r.Client, database, DatabaseAccessFinalizer); err != nil {
				return ctrl.Result{}, err
			}
		}
		if controllerutil.ContainsFinalizer(database, DatabaseFinalizer) {
			if err := r.deleteDatabaseOp(ctx, database); err != nil {
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
				conditions.MarkFalse(database, databasev1alpha1.DatabaseReadyCondition, databasev1alpha1.FailedToCreateDatabaseReason, crhelperTypes.ConditionSeverityError, err.Error())
				if err := patchDatabase(ctx, patchHelper, database); err != nil {
					log.Error(err, "failed to patch Database")
					return ctrl.Result{}, err
				}
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
		conditions.MarkTrue(database, databasev1alpha1.DatabaseReadyCondition)

		// Now we update the DatabaseReady status of DatabaseRequest
		if database.Spec.DatabaseRequest != nil {
			ref := database.Spec.DatabaseRequest

			databaseReq := &databasev1alpha1.DatabaseRequest{}
			if err := r.Get(ctx, client.ObjectKey{
				Namespace: ref.Namespace,
				Name:      ref.Name,
			}, databaseReq); err != nil {
				log.Error(err, "Failed to get database request")
				return ctrl.Result{}, err
			}

			databaseReq.Status.Ready = true
			databaseReq.Status.DatabaseName = database.Name
			if err := r.Status().Update(ctx, databaseReq); err != nil {
				if strings.Contains(err.Error(), genericregistry.OptimisticLockErrorMsg) {
					return reconcile.Result{RequeueAfter: time.Second * 1}, nil
				}
				log.Error(err, "Failed to update DatabaseRequest status")
				return ctrl.Result{}, err
			}
			log.Info("Successfully updated status of DatabaseRequest")
		}
	} else {
		databaseReady = true
		databaseID = database.Spec.ExistingDatabaseID
	}

	database.Status.Ready = databaseReady
	database.Status.DatabaseID = databaseID
	if err := r.Status().Update(ctx, database); err != nil {
		log.Error(err, "Can't update database")
		return ctrl.Result{}, err
	}

	if err := kubernetes.TryAddFinalizer(ctx, r.Client, database, DatabaseFinalizer); err != nil {
		log.Error(err, "Can't update finalizer")
		return ctrl.Result{}, err
	}

	if err := patchDatabase(ctx, patchHelper, database); err != nil {
		if strings.Contains(err.Error(), genericregistry.OptimisticLockErrorMsg) {
			return reconcile.Result{RequeueAfter: time.Second * 1}, nil
		}
		log.Error(err, "failed to patch Database")
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

func patchDatabase(ctx context.Context, patchHelper *patch.Helper, database *databasev1alpha1.Database) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	// A step counter is added to represent progress during the provisioning process (instead we are hiding it during the deletion process).
	conditions.SetSummary(database,
		conditions.WithConditions(
			databasev1alpha1.DatabaseReadyCondition,
		),
		conditions.WithStepCounterIf(database.ObjectMeta.DeletionTimestamp.IsZero()),
		conditions.WithStepCounter(),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		database,
		patch.WithOwnedConditions{Conditions: []crhelperTypes.ConditionType{
			crhelperTypes.ReadyCondition,
			databasev1alpha1.DatabaseReadyCondition,
		},
		},
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.Database{}).
		Complete(r)
}
