package time_wheel

import (
	"fmt"
	"testing"
	"time"
)

func TestBucket(t *testing.T) {
	dummy := newNode(0, nil)

	head := dummy
	head.insert(newNode(1, func() {
		t.Log("first")
	}))
	head.insert(newNode(2, func() {
		t.Log("second")
	}))
	head.insert(newNode(3, func() {
		t.Log("third")
	}))

	dummy.rangeNode(3, func(bucket *wheelNode) (remove bool) {
		bucket.doTask()
		if bucket.remainingRounds == 2 {
			return true
		}
		return false
	})

	dummy.rangeNode(3, func(bucket *wheelNode) (remove bool) {
		bucket.doTask()
		return false
	})
}

func TestWheel(t *testing.T) {
	begin := time.Now()
	tm := initTimeWheelTimer(time.Second, 4, begin)
	idx, rounds := tm.getIdxAndRounds(begin.Add(4 * time.Second))
	fmt.Println("clacRemainingRounds", idx, rounds)
	do := func(count int) {
		err, cancel := tm.SubmitDefer(time.Duration(count)*time.Second, func() {
			fmt.Println(count, " do job", time.Now().Unix()-begin.Unix())
		})
		if err != nil {
			fmt.Println("Err", err)
		}
		if cancel != nil {

			//cancel()
		}
	}
	for i := 1; i <= 50; i++ {
		do(i%9 + 1)
	}

	go tm.run()
	time.Sleep(time.Second * 60)
}

func TestInit(t *testing.T) {
	timer := NewTimeWheelTimer()
	_, _ = timer.SubmitDefer(time.Second, func() {
		fmt.Println("delay 1s")
	})
	_, cancel := timer.SubmitDefer(5*time.Second, func() {
		fmt.Println("delay 5s")
	})
	cancel()

	_, _ = timer.SubmitDefer(6*time.Second, func() {
		fmt.Println("delay 6s")
	})
	_, _ = timer.SubmitDefer(15*time.Second, func() {
		fmt.Println("delay 15s")
	})

	select {}

}

func TestName(t *testing.T) {
	//timer := NewTimeWheelTimer(WithTickDuration(time.Second))
	timer := NewTimeWheelTimer()
	defer timer.Stop()

	_, _ = timer.SubmitDefer(time.Minute, func() {
		t.Log("defer task")
	})
	_, cancel := timer.SubmitDefer(2*time.Minute, func() {
		t.Log("defer task not show")
	})

	cancel()
	select {}

}
