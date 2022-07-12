package consensus

import (
	"context"
	"errors"
	"time"

	eth2client "github.com/attestantio/go-eth2-client"
	v1 "github.com/attestantio/go-eth2-client/api/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samcm/ethereum-metrics-exporter/pkg/exporter/consensus/api"
	"github.com/samcm/ethereum-metrics-exporter/pkg/exporter/consensus/beacon"
	"github.com/samcm/ethereum-metrics-exporter/pkg/exporter/consensus/jobs"
	"github.com/sirupsen/logrus"
)

// Metrics defines a set of metrics for an ethereum consensus node.
type Metrics interface {
	// StartAsync starts the metrics exporter.
	StartAsync(ctx context.Context)
}

type metrics struct {
	log logrus.FieldLogger

	client eth2client.Service

	generalMetrics jobs.General
	syncMetrics    jobs.Sync
	specMetrics    jobs.Spec
	forkMetrics    jobs.Forks
	beaconMetrics  jobs.Beacon
	eventMetrics   jobs.Event
}

// NewMetrics returns a new metrics object.
func NewMetrics(client eth2client.Service, ap api.ConsensusClient, beac beacon.Node, log logrus.FieldLogger, nodeName, namespace string) Metrics {
	constLabels := make(prometheus.Labels)
	constLabels["ethereum_role"] = "consensus"
	constLabels["node_name"] = nodeName

	m := &metrics{
		log:            log,
		client:         client,
		generalMetrics: jobs.NewGeneralJob(beac, log, namespace, constLabels),
		specMetrics:    jobs.NewSpecJob(beac, log, namespace, constLabels),
		syncMetrics:    jobs.NewSyncJob(beac, log, namespace, constLabels),
		forkMetrics:    jobs.NewForksJob(beac, log, namespace, constLabels),
		beaconMetrics:  jobs.NewBeaconJob(client, ap, beac, log, namespace, constLabels),
		eventMetrics:   jobs.NewEventJob(client, beac, log, namespace, constLabels),
	}

	prometheus.MustRegister(m.generalMetrics.NodeVersion)
	prometheus.MustRegister(m.generalMetrics.Peers)

	prometheus.MustRegister(m.syncMetrics.Percentage)
	prometheus.MustRegister(m.syncMetrics.EstimatedHighestSlot)
	prometheus.MustRegister(m.syncMetrics.HeadSlot)
	prometheus.MustRegister(m.syncMetrics.Distance)
	prometheus.MustRegister(m.syncMetrics.IsSyncing)

	prometheus.MustRegister(m.specMetrics.SafeSlotsToUpdateJustified)
	prometheus.MustRegister(m.specMetrics.DepositChainID)
	prometheus.MustRegister(m.specMetrics.ConfigName)
	prometheus.MustRegister(m.specMetrics.MaxValidatorsPerCommittee)
	prometheus.MustRegister(m.specMetrics.SecondsPerEth1Block)
	prometheus.MustRegister(m.specMetrics.BaseRewardFactor)
	prometheus.MustRegister(m.specMetrics.EpochsPerSyncCommitteePeriod)
	prometheus.MustRegister(m.specMetrics.EffectiveBalanceIncrement)
	prometheus.MustRegister(m.specMetrics.MaxAttestations)
	prometheus.MustRegister(m.specMetrics.MinSyncCommitteeParticipants)
	prometheus.MustRegister(m.specMetrics.GenesisDelay)
	prometheus.MustRegister(m.specMetrics.SecondsPerSlot)
	prometheus.MustRegister(m.specMetrics.MaxEffectiveBalance)
	prometheus.MustRegister(m.specMetrics.TerminalTotalDifficulty)
	prometheus.MustRegister(m.specMetrics.TerminalTotalDifficultyTrillions)
	prometheus.MustRegister(m.specMetrics.MaxDeposits)
	prometheus.MustRegister(m.specMetrics.MinGenesisActiveValidatorCount)
	prometheus.MustRegister(m.specMetrics.TargetCommitteeSize)
	prometheus.MustRegister(m.specMetrics.SyncCommitteeSize)
	prometheus.MustRegister(m.specMetrics.Eth1FollowDistance)
	prometheus.MustRegister(m.specMetrics.TerminalBlockHashActivationEpoch)
	prometheus.MustRegister(m.specMetrics.MinDepositAmount)
	prometheus.MustRegister(m.specMetrics.SlotsPerEpoch)
	prometheus.MustRegister(m.specMetrics.PresetBase)

	prometheus.MustRegister(m.forkMetrics.Epochs)
	prometheus.MustRegister(m.forkMetrics.Current)
	prometheus.MustRegister(m.forkMetrics.Activated)

	prometheus.MustRegister(m.beaconMetrics.Attestations)
	prometheus.MustRegister(m.beaconMetrics.Deposits)
	prometheus.MustRegister(m.beaconMetrics.Slashings)
	prometheus.MustRegister(m.beaconMetrics.Transactions)
	prometheus.MustRegister(m.beaconMetrics.VoluntaryExits)
	prometheus.MustRegister(m.beaconMetrics.Slot)
	prometheus.MustRegister(m.beaconMetrics.FinalityCheckpoints)
	prometheus.MustRegister(m.beaconMetrics.ReOrgs)
	prometheus.MustRegister(m.beaconMetrics.ReOrgDepth)
	prometheus.MustRegister(m.beaconMetrics.FinalityCheckpointHash)
	prometheus.MustRegister(m.beaconMetrics.HeadSlotHash)
	prometheus.MustRegister(m.beaconMetrics.ProposerDelay)
	prometheus.MustRegister(m.beaconMetrics.EmptySlots)

	prometheus.MustRegister(m.eventMetrics.Count)
	prometheus.MustRegister(m.eventMetrics.TimeSinceLastEvent)

	return m
}

func (m *metrics) StartAsync(ctx context.Context) {
	go func() {
		if err := m.generalMetrics.Start(ctx); err != nil {
			m.log.Errorf("Failed to start general metrics: %v", err)
		}
	}()

	go func() {
		if err := m.specMetrics.Start(ctx); err != nil {
			m.log.Errorf("Failed to start spec metrics: %v", err)
		}
	}()

	go func() {
		if err := m.syncMetrics.Start(ctx); err != nil {
			m.log.Errorf("Failed to start sync metrics: %v", err)
		}
	}()

	go func() {
		if err := m.forkMetrics.Start(ctx); err != nil {
			m.log.Errorf("Failed to start fork metrics: %v", err)
		}
	}()

	go func() {
		if err := m.beaconMetrics.Start(ctx); err != nil {
			m.log.Errorf("Failed to start beacon metrics: %v", err)
		}
	}()

	go func() {
		if err := m.eventMetrics.Start(ctx); err != nil {
			m.log.Errorf("Failed to start event metrics: %v", err)
		}
	}()

	go m.subscriptionLoop(ctx)
}

func (m *metrics) subscriptionLoop(ctx context.Context) {
	subscribed := false

	for {
		if subscribed && (time.Since(m.eventMetrics.LastEventTime) > (5 * time.Minute)) {
			m.log.
				WithField("last_event_time", m.eventMetrics.LastEventTime.Local().String()).
				Info("Haven't received any events for 5 minutes, re-subscribing")

			subscribed = false
		}

		if !subscribed && m.client != nil {
			if err := m.startSubscriptions(ctx); err != nil {
				m.log.Errorf("Failed to subscribe to eth2 node: %v", err)
			} else {
				subscribed = true
			}
		}

		time.Sleep(60 * time.Second)
	}
}

func (m *metrics) startSubscriptions(ctx context.Context) error {
	m.log.Info("starting subscriptions")

	provider, isProvider := m.client.(eth2client.EventsProvider)
	if !isProvider {
		return errors.New("client does not implement eth2client.Subscriptions")
	}

	topics := []string{}

	for key, supported := range v1.SupportedEventTopics {
		if key == "contribution_and_proof" {
			continue
		}

		if supported {
			topics = append(topics, key)
		}
	}

	if err := provider.Events(ctx, topics, func(event *v1.Event) {
		m.handleEvent(ctx, event)
	}); err != nil {
		return err
	}

	return nil
}

func (m *metrics) handleEvent(ctx context.Context, event *v1.Event) {
	m.beaconMetrics.HandleEvent(ctx, event)
}
