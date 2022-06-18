package gochain

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

func NewHandler(blockchain *Blockchain, nodeID string) http.Handler {
	h := handler{blockchain, nodeID}

	mux := http.NewServeMux()
	mux.HandleFunc("/nodes/register", buildResponse(h.RegisterNode))
	mux.HandleFunc("/nodes/resolve", buildResponse(h.ResolveConflicts))
	mux.HandleFunc("/transactions/new", buildResponse(h.AddTransaction))
	mux.HandleFunc("/mine", buildResponse(h.Mine))
	mux.HandleFunc("/chain", buildResponse(h.Blockchain))
	return mux
}

type handler struct {
	blockchain *Blockchain
	nodeId     string
}

type response struct {
	value      interface{}
	statusCode int
	err        error
}

func buildResponse(h func(io.Writer, *http.Request) response) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := h(w, r)
		msg := resp.value
		if resp.err != nil {
			msg = resp.err.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.statusCode)
		if err := json.NewEncoder(w).Encode(msg); err != nil {
			log.Printf("could not encode response to output: %v", err)
		}
	}
}

func (h *handler) AddTransaction(w io.Writer, r *http.Request) response {
	if r.Method != http.MethodPost {
		return response{
			nil,
			http.StatusMethodNotAllowed,
			fmt.Errorf("method %s not allowd", r.Method),
		}
	}

	log.Printf("transaction to the blockchain...\n")

	var tx Transaction
	err := json.NewDecoder(r.Body).Decode(&tx)
	index := h.blockchain.NewTransaction(tx)

	resp := map[string]string{
		"message": fmt.Sprintf("Transaction will be added to Block %d", index),
	}

	status := http.StatusCreated
	if err != nil {
		status = http.StatusInternalServerError
		log.Printf("there was an error when trying to add a transaction %v\n", err)
		err = fmt.Errorf("fail to add transaction to the blockchain")
	}

	return response{resp, status, err}
}

func (h *handler) Mine(w io.Writer, r *http.Request) response {

	log.Println("Before mining, resolving blockchain differences by consensus")
	h.blockchain.ResolveConflicts()
	transactions := h.blockchain.transactions

	log.Println("Mining some coins")
	var proof int64

	// Improvement (2) (3): Restart the ProofOfWork procedure if to-be-found proof is meaningless.
	for {
		// We run the proof of work algorithm to get the next proof...
		lastBlock := h.blockchain.LastBlock()
		lastProof := lastBlock.Proof

		proof = h.blockchain.ProofOfWork(lastProof)

		// Improvement (2): Restart the ProofOfWork procedure if the local chain has been replaced with an external chain.
		if proof == -1 {
			log.Println("Blockchain updated, proof-of-work restarted")
			continue
		}

		// Improvement (3): Restart the ProofOfWork procedure if proof having been found is obsolete 
		// (i.e., if the local chain has been updated before a proof is found).
		if !h.blockchain.ValidProof(h.blockchain.LastBlock().Proof, proof) {
			log.Println("Proof obsolete, proof-of-work restarted")
			continue
		} 
		break
	}
	
	// Improvement (1): The miner receives the transaction fee as a reward.
	for _, tx := range transactions {
		h.blockchain.NewTransaction(Transaction{Sender: tx.Sender, Recipient: h.nodeId, Amount: tx.Fee, Fee: 0})
	}
	// We must receive a reward for finding the proof.
	// The sender is "0" to signify that this node has mined a new coin.
	newTX := Transaction{Sender: "0", Recipient: h.nodeId, Amount: 1, Fee: 0}
	h.blockchain.NewTransaction(newTX)

	// Forge the new Block by adding it to the chain
	block := h.blockchain.NewBlock(proof, "")
	
	resp := map[string]interface{}{"message": "New Block Forged", "block": block}
	log.Println("New block forged")
	return response{resp, http.StatusOK, nil}
}

func (h *handler) Blockchain(w io.Writer, r *http.Request) response {
	if r.Method != http.MethodGet {
		return response{
			nil,
			http.StatusMethodNotAllowed,
			fmt.Errorf("method %s not allowd", r.Method),
		}
	}

	resp := map[string]interface{}{"chain": h.blockchain.chain, "length": len(h.blockchain.chain)}
	return response{resp, http.StatusOK, nil}
}

func (h *handler) RegisterNode(w io.Writer, r *http.Request) response {
	if r.Method != http.MethodPost {
		return response{
			nil,
			http.StatusMethodNotAllowed,
			fmt.Errorf("method %s not allowd", r.Method),
		}
	}

	log.Println("Adding node to the blockchain")

	var body map[string][]string
	err := json.NewDecoder(r.Body).Decode(&body)

	for _, node := range body["nodes"] {
		h.blockchain.RegisterNode(node)
	}

	resp := map[string]interface{}{
		"message": "New nodes have been added",
		"nodes":   h.blockchain.nodes.Keys(),
	}

	status := http.StatusCreated
	if err != nil {
		status = http.StatusInternalServerError
		err = fmt.Errorf("fail to register nodes")
		log.Printf("there was an error when trying to register a new node %v\n", err)
	}

	return response{resp, status, err}
}

func (h *handler) ResolveConflicts(w io.Writer, r *http.Request) response {
	if r.Method != http.MethodGet {
		return response{
			nil,
			http.StatusMethodNotAllowed,
			fmt.Errorf("method %s not allowd", r.Method),
		}
	}

	log.Println("Resolving blockchain differences by consensus")

	msg := "Our chain is authoritative"
	if h.blockchain.ResolveConflicts() {
		msg = "Our chain was replaced"
	}

	resp := map[string]interface{}{"message": msg, "chain": h.blockchain.chain}
	log.Println(msg)
	return response{resp, http.StatusOK, nil}
}
