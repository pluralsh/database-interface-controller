package databaserequest

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	databasev1alpha1 "github.com/pluralsh/database-interface-api/apis/database/v1alpha1"
)

const (
	DatabaseRequestFinalizer = "pluralsh.database-interface-controller/database-protection"
)

// Reconciler reconciles a DatabaseRequest object
type Reconciler struct {
	client.Client
	Log logr.Logger
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("DatabaseRequest", req.NamespacedName)

	var databaseRequest databasev1alpha1.DatabaseRequest
	if err := r.Get(ctx, req.NamespacedName, &databaseRequest); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if !databaseRequest.DeletionTimestamp.IsZero() {
		databaseRequestCopy := databaseRequest.DeepCopy()
		if controllerutil.ContainsFinalizer(databaseRequestCopy, DatabaseRequestFinalizer) {
			database := &databasev1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{Name: databaseRequest.Status.DatabaseName},
			}
			if err := r.Delete(ctx, database); err != nil {
				log.Error(err, "Error deleting database %s", databaseRequest.Status.DatabaseName)
				return ctrl.Result{}, err
			}
			log.Info("Successfully deleted database: %s", databaseRequest.Status.DatabaseName)
			controllerutil.RemoveFinalizer(databaseRequestCopy, DatabaseRequestFinalizer)
			if err := r.Patch(ctx, databaseRequestCopy, client.MergeFromWithOptions(&databaseRequest, client.MergeFromWithOptimisticLock{})); err != nil {
				log.Error(err, "Error patching database request")
				return ctrl.Result{}, err
			}
			log.Info("Successfully patched database request")
			return ctrl.Result{}, nil
		}
	}

	if !databaseRequest.Status.Ready {
		if databaseRequest.Spec.ExistingDatabaseName == "" {
			databaseClassName := databaseRequest.Spec.DatabaseClassName
			if databaseClassName == "" {
				return ctrl.Result{}, fmt.Errorf("Cannot find database class with the name specified in the database request")
			}

			var databaseClass databasev1alpha1.DatabaseClass
			if err := r.Get(ctx, client.ObjectKey{Name: databaseClassName}, &databaseClass); err != nil {
				log.Error(err, "Can't get database class %s", databaseClassName)
				return ctrl.Result{}, err
			}

			newDatabase := genDatabase(databaseRequest, databaseClass)
			if err := r.Create(ctx, newDatabase); err != nil {
				log.Error(err, "Can't create database")
				return ctrl.Result{}, err
			}
			databaseRequest.Status.Ready = false
		} else {
			databaseRequest.Status.Ready = true
		}

		var database databasev1alpha1.Database
		if err := r.Get(ctx, client.ObjectKey{Name: databaseRequest.Spec.ExistingDatabaseName}, &database); err != nil {
			log.Error(err, "Can't get database")
			return ctrl.Result{}, err
		}
		databaseRequest.Status.DatabaseName = database.Name
		controllerutil.AddFinalizer(&databaseRequest, DatabaseRequestFinalizer)
		if err := r.Update(ctx, &databaseRequest); err != nil {
			log.Error(err, "Can't update database request")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func genDatabase(request databasev1alpha1.DatabaseRequest, class databasev1alpha1.DatabaseClass) *databasev1alpha1.Database {
	name := fmt.Sprintf("%s-%s", class.Name, request.Name)
	return &databasev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: databasev1alpha1.DatabaseSpec{
			DriverName:        class.DriverName,
			DatabaseClassName: class.Name,
			Parameters:        class.Parameters,
			DatabaseRequest: &corev1.ObjectReference{
				Name:      request.Name,
				Namespace: request.Namespace,
			},
		},
		Status: databasev1alpha1.DatabaseStatus{
			Ready:      false,
			DatabaseID: "",
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.DatabaseRequest{}).
		Complete(r)
}
