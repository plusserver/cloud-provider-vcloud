package main

import (
	goflag "flag"
	"fmt"
	"github.com/propero-oss/cloud-provider-vcloud/pkg/cloudproviders/providers/vcloud"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	"k8s.io/klog"
	"k8s.io/kubernetes/cmd/cloud-controller-manager/app"
	cloudcontrollerconfig "k8s.io/kubernetes/cmd/cloud-controller-manager/app/config"
	"k8s.io/kubernetes/cmd/cloud-controller-manager/app/options"
	servicecontroller "k8s.io/kubernetes/pkg/controller/service"
	utilflag "k8s.io/kubernetes/pkg/util/flag"
	"math/rand"
	"net/http"
	"os"
	"time"
)

var (
	versionFlag bool
	version     string
)

//Der Service Controller ist verantwortlich für das Abhören von Ereignissen zum Erstellen, Aktualisieren und Löschen von Diensten.
//Basierend auf dem aktuellen Stand der Services in Kubernetes konfiguriert es Cloud Load Balancer
//(wie ELB, Google LB oder Oracle Cloud Infrastructure LB), um den Zustand der Services in Kubernetes abzubilden.
//Darüber hinaus wird sichergestellt, dass die Service Backends für Cloud loadBalancer auf dem neuesten Stand sind.
func startServiceController(ctx *cloudcontrollerconfig.CompletedConfig, cloud cloudprovider.Interface, stopCh <-chan struct{}) (http.Handler, bool, error) {
	// Start the service controller
	serviceController, err := servicecontroller.New(
		cloud,
		ctx.ClientBuilder.ClientOrDie("service-controller"),
		ctx.SharedInformers.Core().V1().Services(),
		ctx.SharedInformers.Core().V1().Nodes(),
		ctx.ComponentConfig.KubeCloudShared.ClusterName,
	)
	if err != nil {
		// This error shouldn't fail. It lives like this as a legacy.
		klog.Errorf("Failed to start service controller: %v", err)
		return nil, false, nil
	}

	go serviceController.Run(stopCh, int(ctx.ComponentConfig.ServiceController.ConcurrentServiceSyncs))

	return nil, true, nil
}

func newControllerInitializers() map[string]initFunc {
	controllers := map[string]initFunc{}
	controllers["service"] = startServiceController
	return controllers
}

// initFunc is used to launch a particular controller.  It may run additional "should I activate checks".
// Any error returned will cause the controller process to `Fatal`
// The bool indicates whether the controller was enabled.
type initFunc func(ctx *cloudcontrollerconfig.CompletedConfig, cloud cloudprovider.Interface, stop <-chan struct{}) (debuggingHandler http.Handler, enabled bool, err error)

// ControllersDisabledByDefault is the controller disabled default when starting cloud-controller managers.
var ControllersDisabledByDefault = sets.NewString()

func KnownControllers() []string {
	ret := sets.StringKeySet(newControllerInitializers())
	return ret.List()
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	err := goflag.CommandLine.Parse([]string{})
	s, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}

	command := &cobra.Command{
		Use:  "vcloud-cloud-controller-manager",
		Long: `The Cloud controller manager is a daemon that embeds the cloud specific control loops shipped with Kubernetes.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Glog requires this otherwise it complains.
			goflag.CommandLine.Parse(nil)

			// This is a temporary hack to enable proper logging until upstream dependencies
			// are migrated to fully utilize klog instead of glog.
			klogFlags := goflag.NewFlagSet("klog", goflag.ExitOnError)
			klog.InitFlags(klogFlags)

			// Sync the glog and klog flags.
			cmd.Flags().VisitAll(func(f1 *pflag.Flag) {
				f2 := klogFlags.Lookup(f1.Name)
				if f2 != nil {
					value := f1.Value.String()
					f2.Value.Set(value)
				}
			})
		},
		Run: func(cmd *cobra.Command, args []string) {
			if versionFlag {
				fmt.Printf("%s\n", version)
				os.Exit(0)
			}

			utilflag.PrintFlags(cmd.Flags())

			c, err := s.Config(KnownControllers(), ControllersDisabledByDefault.List())
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}

			if err := app.Run(c.Complete(), wait.NeverStop); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
		},
	}

	fs := command.Flags()
	namedFlagSets := s.Flags(KnownControllers(), ControllersDisabledByDefault.List())
	for _, f := range namedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}

	fs.BoolVar(&versionFlag, "version", false, "Print version and exit")

	// TODO: once we switch everything over to Cobra commands, we can go back to calling
	// utilflag.InitFlags() (by removing its pflag.Parse() call). For now, we have to set the
	// normalize func and add the go flag set by hand.
	pflag.CommandLine.SetNormalizeFunc(flag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)
	// utilflag.InitFlags()
	logs.InitLogs()
	defer logs.FlushLogs()

	klog.V(1).Infof("vcloud-cloud-controller-manager version: %s", version)

	s.KubeCloudShared.CloudProvider.Name = vcloud.ProviderName
	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
