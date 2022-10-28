package databaserequest

import (
	"context"
	"fmt"

	crhelperTypes "github.com/pluralsh/controller-reconcile-helper/pkg/types"

	"github.com/go-logr/logr"
	databasev1alpha1 "github.com/pluralsh/database-interface-api/apis/database/v1alpha1"
	"github.com/pluralsh/database-interface-controller/pkg/kubernetes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	DatabaseRequestFinalizer = "pluralsh.database-interface-controller/databaserequest-protection"
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
				ObjectMeta: metav1.ObjectMeta{Name: databaseRequest.Spec.ExistingDatabaseName},
			}
			if err := r.Delete(ctx, database); err != nil {
				log.Error(err, "Error deleting database", "Database", databaseRequest.Spec.ExistingDatabaseName)
				return ctrl.Result{}, err
			}
			log.Info("Successfully deleted database", "Database", databaseRequest.Spec.ExistingDatabaseName)

			return ctrl.Result{}, kubernetes.TryRemoveFinalizer(ctx, r.Client, databaseRequestCopy, DatabaseRequestFinalizer)
		}
		return ctrl.Result{}, nil
	}

	if !databaseRequest.Status.Ready {
		if databaseRequest.Spec.ExistingDatabaseName == "" {
			databaseClassName := databaseRequest.Spec.DatabaseClassName
			if databaseClassName == "" {
				return ctrl.Result{}, fmt.Errorf("Cannot find database class with the name specified in the database request")
			}

			var databaseClass databasev1alpha1.DatabaseClass
			if err := r.Get(ctx, client.ObjectKey{Name: databaseClassName}, &databaseClass); err != nil {
				log.Error(err, "Can't get database class", "databaseClass", databaseClassName)
				return ctrl.Result{}, err
			}

			newDatabase := genDatabase(databaseRequest, databaseClass)
			if err := r.Get(ctx, client.ObjectKey{Name: newDatabase.Name}, &databasev1alpha1.Database{}); err != nil {
				if !apierrors.IsNotFound(err) {
					return ctrl.Result{}, nil
				}
				if err := r.Create(ctx, newDatabase); err != nil {
					log.Error(err, "Can't create database")
					return ctrl.Result{}, err
				}
				log.Info("Successfully created database", "Database", newDatabase.Name)
			}
			databaseRequest.Spec.ExistingDatabaseName = newDatabase.Name
			if err := r.Update(ctx, &databaseRequest); err != nil {
				return ctrl.Result{}, err
			}
			if err := kubernetes.TryAddFinalizer(ctx, r.Client, &databaseRequest, DatabaseRequestFinalizer); err != nil {
				return ctrl.Result{}, err
			}
			databaseRequest.Status.Ready = false
			if err := r.Status().Update(ctx, &databaseRequest); err != nil {
				return ctrl.Result{}, err
			}
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
			DeletionPolicy: class.DeletionPolicy,
		},
		Status: databasev1alpha1.DatabaseStatus{
			Ready:      false,
			DatabaseID: "",
			Conditions: []crhelperTypes.Condition{},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.DatabaseRequest{}).
		Complete(r)
}
