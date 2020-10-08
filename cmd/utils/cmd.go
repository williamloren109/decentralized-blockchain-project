package utils

import (
	"fmt"
	"log"
	"strconv"
	"time"

	logger "github.com/sirupsen/logrus"
	blockchain "github.com/workspace/the-crypto-project/core"
	"github.com/workspace/the-crypto-project/p2p"
	"github.com/workspace/the-crypto-project/wallet"
)

type CommandLine struct {
	Blockchain    *blockchain.Blockchain
	P2p           *p2p.Network
	CloseDbAlways bool
}

type Error struct {
	Code    int
	Message string
}
type BalanceResponse struct {
	Balance   float64
	Address   string
	Timestamp int64
	Error     *Error
}

type SendResponse struct {
	SendTo    string
	SendFrom  string
	Amount    float64
	Timestamp int64
	Error     *Error
}

func (cli *CommandLine) StartNode(listenPort, minerAddress string, miner, fullNode bool, fn func(*p2p.Network)) {
	if miner {
		logger.Infof("Starting Node %s as a MINER\n", listenPort)
	} else {
		logger.Infof("Starting Node %s\n", listenPort)
	}
	if len(minerAddress) > 0 {
		if wallet.ValidateAddress(minerAddress) {
			logger.Info("Mining is ON. Address to receive rewards:", minerAddress)
		} else {
			log.Fatal("Please provide a valid miner address")
		}
	}
	chain := cli.Blockchain.ContinueBlockchain()
	p2p.StartNode(chain, listenPort, minerAddress, miner, fullNode, fn)
}

func (cli *CommandLine) UpdateInstance(InstanceId string, closeDbAlways bool) *CommandLine {
	cli.Blockchain.InstanceId = InstanceId
	if blockchain.Exists(InstanceId) {
		cli.Blockchain = cli.Blockchain.ContinueBlockchain()
	}
	cli.CloseDbAlways = closeDbAlways

	return cli
}
func (cli *CommandLine) Send(from string, to string, amount float64, mineNow bool) SendResponse {

	if !wallet.ValidateAddress(from) {
		fmt.Println("sendFrom address is Invalid ")
		return SendResponse{
			Error: &Error{
				Code:    5028,
				Message: "sendTo address is Invalid",
			},
		}
	}
	if !wallet.ValidateAddress(to) {
		fmt.Println("sendFrom address is Invalid ")
		return SendResponse{
			Error: &Error{
				Code:    5028,
				Message: "sendFrom address is Invalid",
			},
		}
	}

	chain := cli.Blockchain.ContinueBlockchain()
	if cli.CloseDbAlways {
		defer chain.Database.Close()
	}
	utxos := blockchain.UXTOSet{chain}
	cwd := false
	wallets, err := wallet.InitializeWallets(cwd)
	if err != nil {
		chain.Database.Close()
		log.Panic(err)
	}

	wallet := wallets.GetWallet(from)

	tx, err := blockchain.NewTransaction(&wallet, to, amount, &utxos)
	if err != nil {
		fmt.Println(err)
		return SendResponse{
			Error: &Error{
				Code:    5028,
				Message: "failed to execute transaction",
			},
		}
	}
	if mineNow {

		cbTx := blockchain.MinerTx(from, "")
		txs := []*blockchain.Transaction{cbTx, tx}
		block := chain.MineBlock(txs)
		utxos.Update(block)

		if cli.P2p != nil {
			cli.P2p.Blocks <- block
		}
	} else {
		// network.SendTx(network.KnownNodes[0], tx)
		if cli.P2p != nil {
			cli.P2p.Transactions <- tx
		}
		fmt.Println("")
	}
	fmt.Println("Success!")
	return SendResponse{
		SendTo:    to,
		SendFrom:  from,
		Amount:    amount,
		Timestamp: time.Now().Unix(),
	}
}
func (cli *CommandLine) CreateBlockchain(address string) {
	if !wallet.ValidateAddress(address) {
		log.Panic("Invalid address")
	}

	chain := blockchain.InitBlockchain(address, cli.Blockchain.InstanceId)
	if cli.CloseDbAlways {
		defer chain.Database.Close()
	}
	utxos := blockchain.UXTOSet{chain}
	utxos.Compute()
	fmt.Println("Initialized Blockchain Successfully")
}

func (cli *CommandLine) ComputeUTXOs() {
	chain := cli.Blockchain.ContinueBlockchain()

	if cli.CloseDbAlways {
		defer chain.Database.Close()
	}
	utxos := blockchain.UXTOSet{chain}
	utxos.Compute()
	count := utxos.CountTransactions()
	fmt.Printf("Rebuild DONE!!!!, there are %d transactions in the utxos set", count)
}
func (cli *CommandLine) GetBalance(address string) BalanceResponse {
	if !wallet.ValidateAddress(address) {
		log.Panic("Invalid address")
	}
	chain := cli.Blockchain.ContinueBlockchain()
	if cli.CloseDbAlways {
		defer chain.Database.Close()
	}
	balance := float64(0)
	publicKeyHash := wallet.Base58Decode([]byte(address))
	publicKeyHash = publicKeyHash[1 : len(publicKeyHash)-4]
	utxos := blockchain.UXTOSet{chain}

	UTXOs := utxos.FindUnSpentTransactions(publicKeyHash)
	for _, out := range UTXOs {
		balance += out.Value
	}

	fmt.Printf("Balance of %s:%f\n", address, balance)

	return BalanceResponse{
		balance,
		address,
		time.Now().Unix(),
		&Error{},
	}
}

func (cli *CommandLine) CreateWallet() string {
	cwd := false
	wallets, _ := wallet.InitializeWallets(cwd)
	address := wallets.AddWallet()
	wallets.SaveFile(cwd)

	fmt.Println(address)
	return address
}

func (cli *CommandLine) ListAddresses() {
	cwd := false
	wallets, err := wallet.InitializeWallets(cwd)
	if err != nil {
		log.Panic(err)
	}
	addresses := wallets.GetAllAddress()

	for _, address := range addresses {
		fmt.Println(address)
	}
}
func (cli *CommandLine) PrintBlockchain() {
	chain := cli.Blockchain.ContinueBlockchain()
	if cli.CloseDbAlways {
		defer chain.Database.Close()
	}
	iter := chain.Iterator()

	for {
		block := iter.Next()
		fmt.Printf("PrevHash: %x\n", block.PrevHash)
		fmt.Printf("Hash: %x\n", block.Hash)
		fmt.Printf("Height: %d\n", block.Height)
		pow := blockchain.NewProof(block)
		validate := pow.Validate()
		fmt.Printf("Valid: %s\n", strconv.FormatBool(validate))
		for _, tx := range block.Transactions {
			fmt.Println(tx)
		}
		fmt.Println()

		if len(block.PrevHash) == 0 {
			break
		}
	}
}

func (cli *CommandLine) GetBlockchain() []*blockchain.Block {
	var blocks []*blockchain.Block
	chain := cli.Blockchain.ContinueBlockchain()
	if cli.CloseDbAlways {
		defer chain.Database.Close()
	}
	iter := chain.Iterator()

	for {
		block := iter.Next()
		blocks = append(blocks, block)

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return blocks
}

func (cli *CommandLine) GetBlockByHeight(height int) blockchain.Block {
	var block blockchain.Block
	chain := cli.Blockchain.ContinueBlockchain()
	if cli.CloseDbAlways {
		defer chain.Database.Close()
	}
	iter := chain.Iterator()

	for {
		block = *iter.Next()
		if block.Height == height-1 {
			return block
		}
		if len(block.PrevHash) == 0 {
			break
		}
	}

	return block
}
