package tasks

import (
	"log"
	"time"
)

type Order struct {
	ID                string `json:"id"`
	AccumulatedMonths int    `json:"accumulated_months"`
	Status            string `json:"status"`
}

type LeaseAccumulator struct {
	orders []Order
	ticker *time.Ticker
	done   chan bool
}

func NewLeaseAccumulator() *LeaseAccumulator {
	return &LeaseAccumulator{
		orders: []Order{},
		done:   make(chan bool),
	}
}

func (la *LeaseAccumulator) Start() {
	la.ticker = time.NewTicker(30 * 24 * time.Hour)
	go func() {
		for {
			select {
			case <-la.ticker.C:
				if err := la.accumulateMonths(); err != nil {
					log.Printf("Lease accumulation error: %v", err)
				}
			case <-la.done:
				la.ticker.Stop()
				return
			}
		}
	}()
	log.Println("Lease accumulator started - runs monthly")
}

func (la *LeaseAccumulator) Stop() {
	la.done <- true
}

func (la *LeaseAccumulator) accumulateMonths() error {
	count := 0
	for i := range la.orders {
		if la.orders[i].Status == "active" {
			la.orders[i].AccumulatedMonths++
			count++
		}
	}

	log.Printf("Accumulated months for %d active orders", count)

	eligibleOrders := la.getEligibleForTransfer()
	for _, order := range eligibleOrders {
		log.Printf("Order %s is eligible for ownership transfer (accumulated: %d months)", order.ID, order.AccumulatedMonths)
	}

	return nil
}

func (la *LeaseAccumulator) getEligibleForTransfer() []Order {
	var eligible []Order
	for _, order := range la.orders {
		if order.AccumulatedMonths >= 12 && order.AccumulatedMonths < 13 && order.Status == "active" {
			eligible = append(eligible, order)
		}
	}
	return eligible
}

func (la *LeaseAccumulator) AddOrder(order Order) {
	la.orders = append(la.orders, order)
}

func (la *LeaseAccumulator) GetOrders() []Order {
	return la.orders
}
