package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/bombsimon/logrusr/v4"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/prow/pkg/config/secret"
	"sigs.k8s.io/prow/pkg/flagutil"
	configflagutil "sigs.k8s.io/prow/pkg/flagutil/config"
	"sigs.k8s.io/prow/pkg/interrupts"
	"sigs.k8s.io/prow/pkg/logrusutil"
	"sigs.k8s.io/prow/pkg/pjutil"
	"sigs.k8s.io/prow/pkg/pluginhelp/externalplugins"

	"github.com/Huang-Wei/25-kubecon-jp-codegen/pkg/git"
	"github.com/Huang-Wei/25-kubecon-jp-codegen/pkg/prow"
)

type options struct {
	port int

	config                 configflagutil.ConfigOptions
	dryRun                 bool
	github                 flagutil.GitHubOptions
	instrumentationOptions flagutil.InstrumentationOptions
	logLevel               string

	webhookSecretFile string
}

func (o *options) Validate() error {
	for idx, group := range []flagutil.OptionGroup{&o.github} {
		if err := group.Validate(o.dryRun); err != nil {
			return fmt.Errorf("%d: %w", idx, err)
		}
	}

	return nil
}

func gatherOptions() options {
	o := options{config: configflagutil.ConfigOptions{ConfigPath: "/etc/config/config.yaml"}}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.IntVar(&o.port, "port", 8888, "Port to listen on.")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	fs.StringVar(&o.webhookSecretFile, "hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	fs.StringVar(&o.logLevel, "log-level", "debug", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))
	for _, group := range []flagutil.OptionGroup{&o.github, &o.instrumentationOptions, &o.config} {
		group.AddFlags(fs)
	}
	_ = fs.Parse(os.Args[1:])
	return o
}

func main() {
	logrusutil.ComponentInit()
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	logLevel, err := logrus.ParseLevel(o.logLevel)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to parse loglevel")
	}
	logrus.SetLevel(logLevel)
	log := logrus.StandardLogger().WithField("plugin", prow.PluginName)

	if err := secret.Add(o.webhookSecretFile); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	githubClient, err := o.github.GitHubClient(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}
	gitClient, err := o.github.GitClientFactory("", nil, o.dryRun, false)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Git client.")
	}
	interrupts.OnInterrupt(func() {
		if err := gitClient.Clean(); err != nil {
			logrus.WithError(err).Error("Could not clean up git client cache.")
		}
	})

	email, err := githubClient.Email()
	if err != nil {
		log.WithError(err).Fatal("Error getting bot e-mail.")
	}

	botUser, err := githubClient.BotUser()
	if err != nil {
		logrus.WithError(err).Fatal("Error getting bot name.")
	}

	logger := logrusr.New(log)
	gitResourceWorker := git.NewResourceWorker(
		git.WithGC(gitClient),
		git.WithGHC(githubClient),
		git.WithBotUser(botUser),
		git.WithEmail(email),
		git.WithLogger(logger),
	)

	server := prow.NewPlugin(
		secret.GetTokenGenerator(o.webhookSecretFile),
		gitResourceWorker,
	)

	health := pjutil.NewHealthOnPort(o.instrumentationOptions.HealthPort)
	health.ServeReady()

	mux := http.NewServeMux()
	mux.Handle("/", server)
	externalplugins.ServeExternalPluginHelp(mux, log, prow.HelpProvider)
	httpServer := &http.Server{
		Addr:              ":" + strconv.Itoa(o.port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	defer interrupts.WaitForGracefulShutdown()
	interrupts.ListenAndServe(httpServer, 5*time.Second)
}
