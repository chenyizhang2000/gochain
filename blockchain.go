package gochain

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

type BlockchainService interface {
	// Add a new node to the list of nodes
	RegisterNode(address string) bool

	// Determine if a given blockchain is valid
	ValidChain(chain Blockchain) bool

	// This is our Consensus Algorithm, it resolves conflicts by replacing
	// our chain with the longest one in the network.
	ResolveConflicts() bool

	// Create a new Block in the Blockchain
	NewBlock(proof int64, previousHash string) Block

	// Creates a new transaction to go into the next mined Block
	NewTransaction(tx Transaction) int64

	// Returns the last block on the chain
	LastBlock() Block

	// Simple Proof of Work Algorithm:
	// - Find a number p' such that hash(pp') contains leading 4 zeroes, where p is the previous p'
	// - p is the previous proof, and p' is the new proof
	ProofOfWork(lastProof int64)

	// Validates the Proof: Does hash(lastProof, proof) contain 4 leading zeroes?
	VerifyProof(lastProof, proof int64) bool
}

type Block struct {
	Index        int64         `json:"index"`
	Timestamp    int64         `json:"timestamp"`
	Transactions []Transaction `json:"transactions"`
	Proof        int64         `json:"proof"`
	PreviousHash string        `json:"previous_hash"`
}

type Transaction struct {
	Sender    string `json:"sender"`
	Recipient string `json:"recipient"`
	Amount    int64  `json:"amount"`
}

type Blockchain struct {
	chain        []Block
	transactions []Transaction
	nodes        StringSet
}

func (bc *Blockchain) NewBlock(proof int64, previousHash string) Block {
	prevHash := previousHash
	if previousHash == "" {
		prevBlock := bc.chain[len(bc.chain)-1]
		prevHash = computeHashForBlock(prevBlock)
	}

	newBlock := Block{
		Index:        int64(len(bc.chain) + 1),
		Timestamp:    time.Now().UnixNano(),
		Transactions: bc.transactions,
		Proof:        proof,
		PreviousHash: prevHash,
	}

	bc.transactions = nil
	bc.chain = append(bc.chain, newBlock)
	return newBlock
}

func (bc *Blockchain) NewTransaction(tx Transaction) int64 {
	bc.transactions = append(bc.transactions, tx)
	return bc.LastBlock().Index + 1
}

func (bc *Blockchain) LastBlock() Block {
	return bc.chain[len(bc.chain)-1]
}

func (bc *Blockchain) ProofOfWork(lastProof int64) int64 {
	var proof int64 = 0
	for !bc.ValidProof(lastProof, proof) {
		proof += 1
	}
	return proof
}

func (bc *Blockchain) ValidProof(lastProof, proof int64) bool {
	guess := fmt.Sprintf("%d%d", lastProof, proof)
	guessHash := ComputeHashSha256([]byte(guess))
	return guessHash[:4] == "0000"
}

func (bc *Blockchain) ValidChain(chain *[]Block) bool {
	lastBlock := (*chain)[0]
	currentIndex := 1
	for currentIndex < len(*chain) {
		block := (*chain)[currentIndex]
		// Check that the hash of the block is correct
		if block.PreviousHash != computeHashForBlock(lastBlock) {
			return false
		}
		// Check that the Proof of Work is correct
		if !bc.ValidProof(lastBlock.Proof, block.Proof) {
			return false
		}
		lastBlock = block
		currentIndex += 1
	}
	return true
}

func (bc *Blockchain) RegisterNode(address string) bool {
	u, err := url.Parse(address)
	if err != nil {
		return false
	}
	return bc.nodes.Add(u.Host)
}

func (bc *Blockchain) ResolveConflicts() bool {
	curmaxLength := len(bc.chain)
	var authority bool
	authority = true
	tempChain := bc.chain
	for _, node := range bc.nodes.Keys() {
		anotherchain, err := findExternalChain(node)
		if err != nil {
			continue
		}
		if !bc.ValidChain(&anotherchain.Chain) {
			continue
		}
		if anotherchain.Length > curmaxLength {
			curmaxLength = anotherchain.Length
			tempChain = anotherchain.Chain
		}
	}
	bc.chain = tempChain
	return (!authority)
}

func NewBlockchain() *Blockchain {
	newBlockchain := &Blockchain{
		chain:        make([]Block, 0),
		transactions: make([]Transaction, 0),
		nodes:        NewStringSet(),
	}
	// Initial, sentinel block
	newBlockchain.NewBlock(100, "1")
	return newBlockchain
}

func computeHashForBlock(block Block) string {
	var buf bytes.Buffer
	// Data for binary.Write must be a fixed-size value or a slice of fixed-size values,
	// or a pointer to such data.
	jsonblock, marshalErr := json.Marshal(block)
	if marshalErr != nil {
		log.Fatalf("Could not marshal block: %s", marshalErr.Error())
	}
	hashingErr := binary.Write(&buf, binary.BigEndian, jsonblock)
	if hashingErr != nil {
		log.Fatalf("Could not hash block: %s", hashingErr.Error())
	}
	return ComputeHashSha256(buf.Bytes())
}

type blockchainInfo struct {
	Length int     `json:"length"`
	Chain  []Block `json:"chain"`
}

func findExternalChain(address string) (blockchainInfo, error) {
	response, err := http.Get(fmt.Sprintf("http://%s/chain", address))
	if err == nil && response.StatusCode == http.StatusOK {
		var bi blockchainInfo
		if err := json.NewDecoder(response.Body).Decode(&bi); err != nil {
			return blockchainInfo{}, err
		}
		return bi, nil
	}
	return blockchainInfo{}, err
}
