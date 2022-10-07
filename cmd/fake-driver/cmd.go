package main

import (
	"context"
	"flag"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/pluralsh/database-interface-controller/pkg/provisioner"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

const provisionerName = "fake.database.plural.sh"

var (
	driverAddress = "unix:///var/lib/database/database.sock"
)

var cmd = &cobra.Command{
	Use:           "fake-database-driver",
	Short:         "K8s database driver for Fake database",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return run(cmd.Context(), args)
	},
	DisableFlagsInUseLine: true,
}

func init() {
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	flag.Set("alsologtostderr", "true")
	kflags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(kflags)

	persistentFlags := cmd.PersistentFlags()
	persistentFlags.AddGoFlagSet(kflags)

	stringFlag := persistentFlags.StringVarP

	stringFlag(&driverAddress,
		"driver-addr",
		"d",
		driverAddress,
		"path to unix domain socket where driver should listen")
	viper.BindPFlags(cmd.PersistentFlags())
	cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if viper.IsSet(f.Name) && viper.GetString(f.Name) != "" {
			cmd.PersistentFlags().Set(f.Name, viper.GetString(f.Name))
		}
	})
}

func run(ctx context.Context, args []string) error {
	identityServer, bucketProvisioner := NewDriver(provisionerName)
	server, err := provisioner.NewDefaultProvisionerServer(driverAddress,
		identityServer,
		bucketProvisioner)
	if err != nil {
		return err
	}
	return server.Run(ctx)
}
