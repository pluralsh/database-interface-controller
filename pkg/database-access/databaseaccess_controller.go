package databaseaccess

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	databasev1alpha1 "github.com/pluralsh/database-interface-api/apis/database/v1alpha1"
	databasespec "github.com/pluralsh/database-interface-api/spec"
	databasectrl "github.com/pluralsh/database-interface-controller/pkg/database"
	"github.com/pluralsh/database-interface-controller/pkg/kubernetes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconciler reconciles a DatabaseAccess object
type Reconciler struct {
	client.Client
	Log logr.Logger

	DriverName        string
	ProvisionerClient databasespec.ProvisionerClient
}

const (
	SecretFinalizer         = "pluralsh.database-interface-controller/secret-protection"
	DatabaseAccessFinalizer = "pluralsh.database-interface-controller/databaseaccess-protection"
)

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("DatabaseAccess", req.NamespacedName)

	databaseAccess := &databasev1alpha1.DatabaseAccess{}
	if err := r.Get(ctx, req.NamespacedName, databaseAccess); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !databaseAccess.GetDeletionTimestamp().IsZero() {
		if err := r.deleteDatabaseAccessOp(ctx, databaseAccess); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if databaseAccess.Status.AccessGranted && databaseAccess.Status.AccountID != "" {
		log.Info("DatabaseAccess already exists")
		return ctrl.Result{}, nil
	}

	databaseRequestName := databaseAccess.Spec.DatabaseRequestName
	databaseAccessClassName := databaseAccess.Spec.DatabaseAccessClassName
	log.Info("Add DatabaseAccess")

	secretCredName := databaseAccess.Spec.CredentialsSecretName
	if secretCredName == "" {
		return ctrl.Result{}, errors.New("CredentialsSecretName not defined in the DatabaseAccess")
	}

	databaseAccessClass := &databasev1alpha1.DatabaseAccessClass{}
	if err := r.Get(ctx, client.ObjectKey{Name: databaseAccessClassName}, databaseAccessClass); err != nil {
		log.Error(err, "Failed to get DatabaseAccessClass")
		return ctrl.Result{}, err
	}
	if !strings.EqualFold(databaseAccessClass.DriverName, r.DriverName) {
		log.Info("Skipping databaseaccess for driver")
		return ctrl.Result{}, nil
	}

	namespace := databaseAccess.ObjectMeta.Namespace
	databaseRequest := &databasev1alpha1.DatabaseRequest{}
	if err := r.Get(ctx, client.ObjectKey{Name: databaseRequestName, Namespace: namespace}, databaseRequest); err != nil {
		log.Error(err, "Failed to get DatabaseRequest")
		return ctrl.Result{}, err
	}
	if databaseRequest.Status.DatabaseName == "" || databaseRequest.Status.Ready != true {
		err := errors.New("DatabaseName cannot be empty or NotReady in databaseRequest")
		return ctrl.Result{}, err
	}
	if databaseAccess.Status.AccessGranted == true {
		log.Info("AccessAlreadyGranted")
		return ctrl.Result{}, nil
	}

	database := &databasev1alpha1.Database{}
	if err := r.Get(ctx, client.ObjectKey{Name: databaseRequest.Status.DatabaseName}, database); err != nil {
		log.Error(err, "Failed to get Database")
		return ctrl.Result{}, err
	}
	if database.Status.Ready != true || database.Status.DatabaseID == "" {
		err := errors.New("DatabaseAccess can't be granted to database not in Ready state and without a databaseID")
		return ctrl.Result{}, err
	}

	accountName := fmt.Sprintf("%s-%s", "account", databaseAccess.Name)
	grantAccessReq := &databasespec.DriverGrantDatabaseAccessRequest{
		DatabaseId:         database.Status.DatabaseID,
		Name:               accountName,
		AuthenticationType: 0,
		Parameters:         databaseAccessClass.Parameters,
	}

	rsp, err := r.ProvisionerClient.DriverGrantDatabaseAccess(ctx, grantAccessReq)
	if err != nil {
		if status.Code(err) != codes.AlreadyExists {
			log.Error(err, "Failed to grant access")
			return ctrl.Result{}, err
		}

	}
	credentials := rsp.Credentials

	credentialSecret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Name: secretCredName, Namespace: namespace}, credentialSecret); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get credential secret")
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:       secretCredName,
				Namespace:  namespace,
				Finalizers: []string{SecretFinalizer},
			},
			StringData: credentials["cred"].Secrets,
			Type:       corev1.SecretTypeOpaque,
		}); err != nil {
			log.Error(err, "Failed to create secret")
			return ctrl.Result{}, err
		}
	}

	if err := kubernetes.TryAddFinalizer(ctx, r.Client, database, databasectrl.DatabaseAccessFinalizer); err != nil {
		return ctrl.Result{}, err
	}
	if err := kubernetes.TryAddFinalizer(ctx, r.Client, databaseAccess, DatabaseAccessFinalizer); err != nil {
		return ctrl.Result{}, err
	}

	databaseAccess.Status.AccountID = rsp.AccountId
	databaseAccess.Status.AccessGranted = true

	if err := r.Status().Update(ctx, databaseAccess); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) deleteDatabaseAccessOp(ctx context.Context, databaseAccess *databasev1alpha1.DatabaseAccess) error {
	credSecretName := databaseAccess.Spec.CredentialsSecretName
	credentialSecret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Name: credSecretName, Namespace: databaseAccess.Namespace}, credentialSecret); err != nil {
		return err
	}
	if err := r.Delete(ctx, credentialSecret); err != nil {
		return err
	}

	if err := kubernetes.TryRemoveFinalizer(ctx, r.Client, credentialSecret, SecretFinalizer); err != nil {
		return err
	}

	if err := kubernetes.TryRemoveFinalizer(ctx, r.Client, databaseAccess, DatabaseAccessFinalizer); err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.DatabaseAccess{}).
		Complete(r)
}
