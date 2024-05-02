package main

import (
	"context"
	"fmt"
	// "github.com/alitto/pond"
	"github.com/antithesishq/antithesis-sdk-go/assert"
	"github.com/antithesishq/antithesis-sdk-go/lifecycle"
	"github.com/antithesishq/antithesis-sdk-go/random"
	sdk "github.com/formancehq/formance-sdk-go/v2"
	"github.com/formancehq/formance-sdk-go/v2/pkg/models/operations"
	"github.com/formancehq/formance-sdk-go/v2/pkg/models/shared"
	"github.com/formancehq/stack/libs/go-libs/pointer"
	// "go.uber.org/atomic"
	// "math"
	"math/big"
	"net/http"
	"os"
	"time"
)

type Details map[string]any

func main() {
	ctx := context.Background()
	client := sdk.New(
		sdk.WithServerURL("http://gateway:8080"),
		sdk.WithClient(&http.Client{
			Timeout: 10 * time.Second,
			//Transport: httpclient.NewDebugHTTPTransport(http.DefaultTransport),
		}),
	)

	waitServicesReady(ctx, client)

	runWorkload(ctx, client)
}

func waitServicesReady(ctx context.Context, client *sdk.Formance) {
	fmt.Println("Waiting for services to be ready")
	waitingServicesCtx, cancel := context.WithDeadline(ctx, time.Now().Add(30*time.Second))
	defer cancel()

	for {
		select {
		case <-time.After(time.Second):
			fmt.Println("Trying to get info of the ledger...")
			_, err := client.Ledger.GetInfo(ctx)
			if err != nil {
				fmt.Printf("error pinging ledger: %s\r\n", err)
				continue
			}
			return
		case <-waitingServicesCtx.Done():
			fmt.Printf("timeout waiting for services to be ready\r\n")
			os.Exit(1)
		}
	}
}

func randomBigInt() *big.Int {
	v := random.GetRandom()
	ret := big.NewInt(0)
	ret.SetString(fmt.Sprintf("%d", v), 10)
	return ret
}

func randomTransferAmountBigInt() *big.Int {
	// range:  a ... b inclusive   10 ... 100
	// extent: (b - a) + 1     (100 - 10) + 1 => 91
	// large random number: num (from SDK)
	// c: (num % extent) 0..(extent-1)
	// result = a + c

	low := uint64(1)
	high := uint64(100)
	extent := uint64((high - low) + 1)
	v := random.GetRandom()
	result := int64(low + (v % extent))
	return big.NewInt(result)
}

func runWorkload(ctx context.Context, client *sdk.Formance) {
	fmt.Println("Creating ledger...")
	_, err := client.Ledger.V2CreateLedger(ctx, operations.V2CreateLedgerRequest{
		Ledger: "default",
	})
	if !assert.Always(err == nil, "ledger should have been created", Details{
		"error": fmt.Sprintf("%+v\n", err),
	}) {
		return
	}

	numAccounts := 10
	balance := big.NewInt(1000)
	totalBalance := big.NewInt(10000)

	fundAccounts(ctx, client, numAccounts, balance)

	// signals that the system is up and running
	lifecycle.SetupComplete(Details{"Ledger": "Available"})

	fmt.Println("Checking balance of 'world'...")
	account, err := client.Ledger.V2GetAccount(ctx, operations.V2GetAccountRequest{
		Address: "world",
		Expand:  pointer.For("volumes"),
		Ledger:  "default",
	})
	if !assert.Always(err == nil, "we should be able to query account 'world'", Details{
		"error": fmt.Sprintf("%+v\n", err),
	}) {
		return
	}

	output := account.V2AccountResponse.Data.Volumes["USD/2"].Output
	if !assert.Always(output != nil, "Expect output of world for USD/2 to be not empty", Details{}) {
		return
	}
	fmt.Printf("Expect output of world to be %s and got %d\r\n", totalBalance, output)
	assert.Always(
		output.Cmp(totalBalance) == 0,
		"output of 'world' should match",
		Details{
			"output": output,
		},
	)

	for i := 0; i < 1000; i++ {
		if i % 10 == 0 {
			checkAllBalances(ctx, client, numAccounts, totalBalance)
		}
		runTrade(ctx, client, numAccounts)
	}

	fmt.Printf("WORKLOAD COMPLETED\r\n")
}

func checkAllBalances(ctx context.Context, client *sdk.Formance, numAccounts int, totalBalance *big.Int) {
	actualBalance := big.NewInt(0)
	var allBalances []string

	for i := 0; i < numAccounts; i++ {
		accountName := fmt.Sprintf("account:%s", fmt.Sprint(int64(i)))

		account, err := client.Ledger.V2GetAccount(ctx, operations.V2GetAccountRequest{
			Address: accountName,
			Expand:  pointer.For("volumes"),
			Ledger:  "default",
		})

		if err == nil {
			balance := account.V2AccountResponse.Data.Volumes["USD/2"].Balance

			allBalances = append(allBalances, fmt.Sprintf("%s=%d", accountName, balance))

			actualBalance = actualBalance.Add(actualBalance, balance)
		}
	}

	fmt.Printf("ALL BALANCES: %v\r\n", allBalances)
	fmt.Printf("VALIDATE: Expected total balance to be %d and got %d\r\n", totalBalance, actualBalance)
	assert.Always(
		actualBalance.Cmp(totalBalance) == 0,
		"actual balance should match total",
		Details{
			"balance": actualBalance,
		},
	)
}

func runTrade(ctx context.Context, client *sdk.Formance, numAccounts int) {
	source := random.GetRandom() % uint64(numAccounts)
	dest := random.GetRandom() % uint64(numAccounts)

	amount := randomTransferAmountBigInt()

	fmt.Printf("TRANSFER: %d from %d to %d\r\n", amount, source, dest)
	_, err := client.Ledger.V2CreateTransaction(ctx, operations.V2CreateTransactionRequest{
		V2PostTransaction: shared.V2PostTransaction{
			Postings: []shared.V2Posting{{
				Amount:      amount,
				Asset:       "USD/2",
				Destination: fmt.Sprintf("account:%s", fmt.Sprint(dest)),
				Source:      fmt.Sprintf("account:%s", fmt.Sprint(source)),
			}},
		},
		Ledger: "default",
	})
	assert.Always(err == nil, "running trade should always return a nil error", Details{
		"error": fmt.Sprintf("%+v\n", err),
	})
}

func fundAccounts(ctx context.Context, client *sdk.Formance, numAccounts int, balance *big.Int) {
	fmt.Printf("Fund %d accounts with %d each\r\n", numAccounts, balance)

	for i := 0; i < numAccounts; i++ {
		_, err := client.Ledger.V2CreateTransaction(ctx, operations.V2CreateTransactionRequest{
			V2PostTransaction: shared.V2PostTransaction{
				Postings: []shared.V2Posting{{
					Amount:      balance,
					Asset:       "USD/2",
					Destination: fmt.Sprintf("account:%s", fmt.Sprint(int64(i))),
					Source:      "world",
				}},
			},
			Ledger: "default",
		})
		assert.Always(err == nil, "funding accounts should always complete without an error", Details{
			"error": fmt.Sprintf("%+v\n", err),
		})
	}

	fmt.Printf("Finished funding all accounts\r\n")
}
