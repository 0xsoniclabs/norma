package main

import (
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	netmon "github.com/0xsoniclabs/norma/driver/monitoring/network"
	nodemon "github.com/0xsoniclabs/norma/driver/monitoring/node"
	"github.com/0xsoniclabs/norma/driver/network/local"
	"golang.org/x/exp/constraints"
)

// progressLogger is a helper struct that logs the progress of the network.
// It lists nodes and logs the progress of the network periodically.
type progressLogger struct {
	monitor *monitoring.Monitor
	stop    chan<- bool
	done    <-chan bool
}

// startProgressLogger starts a progress logger that logs the progress of the network.
func startProgressLogger(monitor *monitoring.Monitor, net *local.LocalNetwork) *progressLogger {
	stop := make(chan bool)
	done := make(chan bool)

	activeNodes := &activeNodes{
		data: make(map[driver.NodeID]struct{}),
	}
	net.RegisterListener(activeNodes)
	for _, node := range net.GetActiveNodes() {
		activeNodes.AfterNodeCreation(node)
	}

	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				logState(monitor, activeNodes)
			}
		}
	}()

	return &progressLogger{
		monitor,
		stop,
		done,
	}
}

type activeNodes struct {
	data  map[driver.NodeID]struct{}
	mutex sync.Mutex
}

func (l *activeNodes) AfterNodeCreation(node driver.Node) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.data[driver.NodeID(node.GetLabel())] = struct{}{}
}

func (l *activeNodes) BeforeNodeRemoval(node driver.Node) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	delete(l.data, driver.NodeID(node.GetLabel()))
}

func (l *activeNodes) AfterApplicationCreation(app driver.Application) {
	// noop
}

func (l *activeNodes) containsId(id driver.NodeID) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	_, exists := l.data[id]
	return exists
}

func (l *progressLogger) shutdown() {
	close(l.stop)
	<-l.done
}

func logState(monitor *monitoring.Monitor, nodes *activeNodes) {
	numNodes := getNumNodes(monitor)
	blockStatuses := getBlockStatuses(monitor, nodes)
	txPers := getTxPerSec(monitor, nodes)
	txs := getNumTxs(monitor)
	gas := getGasUsed(monitor)
	stakes := getValidatorStakes(monitor)
	processingTimes := getBlockProcessingTimes(monitor, nodes)

	slog.Info("progress update",
		"nodes", numNodes,
		"epc/blk", blockStatuses,
		"tx/s", txPers,
		"txs", txs,
		"gas", gas,
		"stake", stakes,
		"blk t", processingTimes)
}

func getNumNodes(monitor *monitoring.Monitor) string {
	data, exists := monitoring.GetData(monitor, monitoring.Network{}, netmon.NumberOfNodes)
	return getLastValAsString[monitoring.Time, int](exists, data)
}

func getNumTxs(monitor *monitoring.Monitor) string {
	data, exists := monitoring.GetData(monitor, monitoring.Network{}, netmon.BlockNumberOfTransactions)
	return getLastValAsString[monitoring.BlockNumber, int](exists, data)
}

func getTxPerSec(monitor *monitoring.Monitor, nodes *activeNodes) []string {
	metric := nodemon.TransactionsThroughput
	return getLastValAllSubjects[monitoring.BlockNumber, float32](monitor, metric, nodes)
}

func getGasUsed(monitor *monitoring.Monitor) string {
	data, exists := monitoring.GetData(monitor, monitoring.Network{}, netmon.BlockGasUsed)
	return getLastValAsString[monitoring.BlockNumber, int](exists, data)
}

func getBlockStatuses(monitor *monitoring.Monitor, nodes *activeNodes) []string {
	metric := nodemon.NodeBlockStatus
	return getLastValAllSubjects[
		monitoring.Time,
		monitoring.BlockStatus,
		monitoring.Series[monitoring.Time, monitoring.BlockStatus]](
		monitor, metric, nodes)
}

func getBlockProcessingTimes(monitor *monitoring.Monitor, nodes *activeNodes) []string {
	metric := nodemon.BlockEventAndTxsProcessingTime
	return getLastValAllSubjects[
		monitoring.BlockNumber,
		time.Duration,
		monitoring.Series[monitoring.BlockNumber, time.Duration]](
		monitor, metric, nodes)
}

func getValidatorStakes(monitor *monitoring.Monitor) []string {
	metric := netmon.ValidatorStake
	subjects := monitoring.GetSubjects(monitor, metric)
	sort.Slice(subjects, func(i, j int) bool {
		iID, iOK := validatorIDFromMetricSubject(subjects[i])
		jID, jOK := validatorIDFromMetricSubject(subjects[j])
		if iOK && jOK {
			return iID < jID
		}
		return subjects[i] < subjects[j]
	})

	stakes := make([]*big.Int, 0, len(subjects))
	totalStake := big.NewInt(0)
	for _, subject := range subjects {
		data, exists := monitoring.GetData(monitor, subject, metric)
		stakeStr := getLastValAsString[monitoring.Time, string](exists, data)
		stake, ok := new(big.Int).SetString(stakeStr, 10)
		if !ok {
			stakes = append(stakes, nil)
			continue
		}
		stakes = append(stakes, stake)
		totalStake.Add(totalStake, stake)
	}

	res := make([]string, 0, len(stakes))
	if totalStake.Sign() == 0 {
		for range stakes {
			res = append(res, "N/A")
		}
		return res
	}

	for _, stake := range stakes {
		if stake == nil {
			res = append(res, "N/A")
			continue
		}
		// Rounded integer percentage: (stake*100 + total/2) / total.
		numerator := new(big.Int).Mul(stake, big.NewInt(100))
		halfTotal := new(big.Int).Div(new(big.Int).Set(totalStake), big.NewInt(2))
		numerator.Add(numerator, halfTotal)
		percent := new(big.Int).Div(numerator, totalStake)
		res = append(res, percent.String()+"%")
	}
	return res
}

func validatorIDFromMetricSubject(subject monitoring.Node) (int, bool) {
	prefix := "validator-"
	name := string(subject)
	if !strings.HasPrefix(name, prefix) {
		return 0, false
	}
	id, err := strconv.Atoi(strings.TrimPrefix(name, prefix))
	if err != nil {
		return 0, false
	}
	return id, true
}

func getLastValAllSubjects[K constraints.Ordered, T any, X monitoring.Series[K, T]](
	monitor *monitoring.Monitor,
	metric monitoring.Metric[monitoring.Node, X],
	activeNodes *activeNodes) []string {

	nodes := monitoring.GetSubjects(monitor, metric)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })

	res := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if exists := activeNodes.containsId(driver.NodeID(node)); exists {
			data, exists := monitoring.GetData(monitor, node, metric)
			d := getLastValAsString[K, T](exists, data)
			res = append(res, d)
		}
	}
	return res
}

func getLastValAsString[K constraints.Ordered, T any](exists bool, series monitoring.Series[K, T]) string {
	if !exists || series == nil {
		return "N/A"
	}
	point := series.GetLatest()
	if point == nil {
		return "N/A"
	}
	return fmt.Sprintf("%v", point.Value)
}
