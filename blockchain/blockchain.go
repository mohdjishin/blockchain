package blockchain

import (
	"encoding/hex"
	"fmt"
	"os"
	"runtime"

	"github.com/dgraph-io/badger"
)

const (
	dbPath = "./tmp/blocks"

	dbFile      = "./tmp/blocks/MANIFEST"
	genesisData = "First Transaction from Genesis"
)

type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

type BlockChainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

func DBexists() bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}
	return true
}

func InitBlockChian(address string) *BlockChain {

	var lastHash []byte

	if DBexists() {
		fmt.Println("Blockchian already exists!")
		runtime.Goexit()
	}

	opts := badger.DefaultOptions(dbPath)
	opts.Dir = dbPath
	opts.ValueDir = dbPath

	db, err := badger.Open(opts)
	Handle(err)

	db.Update(func(txn *badger.Txn) error {
		cbtx := CoinbaseTx(address, genesisData)
		genesis := Genesis(cbtx)
		fmt.Println("Genesis created")
		err = txn.Set(genesis.Hash, genesis.Serialize())
		Handle(err)
		err = txn.Set([]byte("lh"), genesis.Hash)

		lastHash = genesis.Hash

		return err

	})
	Handle(err)
	blockchain := BlockChain{lastHash, db}
	return &blockchain

}

func ContinueBlockChain(address string) *BlockChain {
	if DBexists() == false {
		fmt.Println("No Existing Blockchain found,Create one!")
		runtime.Goexit()
	}
	var lastHash []byte
	opts := badger.DefaultOptions(dbPath)
	opts.Dir = dbPath
	opts.ValueDir = dbPath

	db, err := badger.Open(opts)
	Handle(err)
	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, err = item.ValueCopy(nil)
		return err
	})
	Handle(err)
	chain := &BlockChain{LastHash: lastHash, Database: db}
	return chain
}

func (chain *BlockChain) AddBlock(transcations []*Transaction) {

	var lastHash []byte

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, err = item.ValueCopy(nil)
		return err
	})
	Handle(err)

	newBlock := CreateBlock(transcations, lastHash)
	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.Serialize())

		Handle(err)
		err = txn.Set([]byte("lh"), newBlock.Hash)
		chain.LastHash = newBlock.Hash
		return err
	})
	Handle(err)
	// prevBlock := chain.Blocks[len(chain.Blocks)-1]
	// new := CreateBlock(data, prevBlock.Hash)
	// chain.Blocks = append(chain.Blocks, new)
}
func (chain *BlockChain) Iterator() *BlockChainIterator {
	iter := &BlockChainIterator{chain.LastHash, chain.Database}
	return iter
}

func (iter *BlockChainIterator) Next() *Block {
	var block *Block
	err := iter.Database.View(func(txn *badger.Txn) error {

		item, err := txn.Get(iter.CurrentHash)
		Handle(err)
		encodedBlock, err := item.ValueCopy(nil)
		block = Deserialize(encodedBlock)
		return err
	})
	Handle(err)
	iter.CurrentHash = block.PrevHash

	return block
}

func (chain *BlockChain) FindUnspentTransactions(address string) []Transaction {
	var unspendTxs []Transaction
	spendTxs := make(map[string][]int)
	iter := chain.Iterator()
	for {
		block := iter.Next()

		for _, tx := range block.Transaction {
			txId := hex.EncodeToString(tx.ID)
		Outputs:
			for outIdx, out := range tx.Output {
				if spendTxs[txId] != nil {
					for _, spentOut := range spendTxs[txId] {
						if spentOut == outIdx {
							continue Outputs
						}

					}
				}
				if out.CanBeUnlocked(address) {
					unspendTxs = append(unspendTxs, *tx)
				}

			}
			if tx.IsCoinBase() == false {
				for _, in := range tx.Inputs {
					if in.CanUnlock(address) {
						inTxId := hex.EncodeToString(in.ID)
						spendTxs[inTxId] = append(spendTxs[inTxId], in.Out)
					}

				}
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}
	return unspendTxs
}

func (chain *BlockChain) FindUTXO(address string) []TxOutput {
	var UTXOs []TxOutput
	unspentTransaction := chain.FindUnspentTransactions(address)

	for _, tx := range unspentTransaction {

		for _, out := range tx.Output {
			if out.CanBeUnlocked(address) {
				UTXOs = append(UTXOs, out)
			}
		}

	}

	return UTXOs
}
func (chain *BlockChain) FindSpendableOutputs(address string, amount int) (int, map[string][]int) {

	unspentOuts := make(map[string][]int)
	unspentTxs := chain.FindUnspentTransactions(address)
	accumulated := 0

Work:
	for _, tx := range unspentTxs {
		txId := hex.EncodeToString(tx.ID)
		for outIdx, out := range tx.Output {
			if out.CanBeUnlocked(address) && accumulated < amount {
				accumulated += out.Value
				unspentOuts[txId] = append(unspentOuts[txId], outIdx)

				if accumulated >= amount {
					break Work
				}
			}

		}

	}

	return accumulated, unspentOuts
}
