package time_wheel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

const maxRangeCount = 100000

var (
	TooCloseCurrentTimeError = errors.New("delay time is small than tick duration")
	TaskIsNilError           = errors.New("task is nil")
	cancelTask               = func() {}
)

type (
	option struct {
		tickDuration time.Duration
		wheelLength  int64
	}
	optionFunc func(opt *option)

	TimeWheelTimer struct {
		seq          *uint64 // 每遍历一个 bucket ，seq 就会加 1 , 用于 bucket 执行时校验是否应过期
		tickDuration time.Duration
		timeWheel    []*wheelBucket
		begin        time.Time
		*shutDown
	}
	shutDown struct {
		context.Context
		context.CancelFunc
	}

	// wheelNode 双向链表
	wheelNode struct {
		prev, next      *wheelNode
		remainingRounds int64
		task            atomic.Value
	}
	// wheelBucket 时间轮上的刻度
	wheelBucket struct {
		*wheelNode
		seq  *uint64 // 当前 bucket 每执行一次，seq 就会更新成和 TimeWheelTimer.seq 相同的值
		lock sync.Mutex
	}
)

func initShutDown() *shutDown {
	ctx, cancel := context.WithCancel(context.Background())
	return &shutDown{
		Context:    ctx,
		CancelFunc: cancel,
	}
}

func (bucket *wheelBucket) getSeq() uint64 {
	return atomic.LoadUint64(bucket.seq)
}

func (t *TimeWheelTimer) getSeq() uint64 {
	return atomic.LoadUint64(t.seq)
}

func (t *TimeWheelTimer) incrSeq() uint64 {
	return atomic.AddUint64(t.seq, 1)
}
func (bucket *wheelBucket) setSeq(seq uint64) {
	atomic.StoreUint64(bucket.seq, seq)
}

func initBucket() *wheelBucket {
	return &wheelBucket{
		wheelNode: newNode(-1, nil),
		seq:       new(uint64),
	}
}

func (o *option) checkParam() bool {
	return !(o.tickDuration <= 0 || o.wheelLength <= 0)
}
func WithTickDuration(tickDuration time.Duration) optionFunc {
	return func(opt *option) {
		opt.tickDuration = tickDuration
	}
}
func WithWheelLength(wheelLength int64) optionFunc {
	return func(opt *option) {
		opt.wheelLength = wheelLength
	}
}

func initTimeWheelTimer(tickDuration time.Duration, wheelLength int64, begin time.Time) *TimeWheelTimer {
	initTimeWheel := func() (list []*wheelBucket) {
		list = make([]*wheelBucket, wheelLength)
		for i := range list {
			list[i] = initBucket()
		}
		return
	}
	return &TimeWheelTimer{
		tickDuration: tickDuration,
		begin:        begin,
		seq:          new(uint64),
		shutDown:     initShutDown(),
		timeWheel:    initTimeWheel(),
	}
}

func NewTimeWheelTimer(options ...optionFunc) *TimeWheelTimer {
	var opt = option{
		tickDuration: time.Second,
		wheelLength:  60,
	}
	for _, f := range options {
		f(&opt)
	}
	if !opt.checkParam() {
		panic("please check param")
	}
	timeWheel := initTimeWheelTimer(opt.tickDuration, opt.wheelLength, time.Now())
	go timeWheel.run()
	return timeWheel
}

func (t *TimeWheelTimer) SubmitDefer(delay time.Duration, task func()) (err error, cancel func()) {
	if task == nil {
		return TaskIsNilError, nil
	}
	if delay < t.tickDuration {
		return TooCloseCurrentTimeError, nil
	}
	var (
		idx, rounds = t.getIdxAndRounds(time.Now().Add(delay))
		newItem     = newNode(rounds, task)
	)

	t.submit(t.timeWheel[idx], newItem)
	return nil, newItem.cancel
}

// submit 将 node 添加进队列，如果该 node 已经过期，则立即执行
func (t *TimeWheelTimer) submit(bucket *wheelBucket, node *wheelNode) {
	// 抢到锁之前获取的 seq 如果和抢到锁后的 seq 不相同，说明在等待锁的这段事件内，该 bucket 已经被遍历过
	// 此时如果直接把 node 加入队列则该任务无法及时执行，所以放弃加队列，直接异步执行
	seq := bucket.getSeq()
	bucket.LockFunc(func(header *wheelNode) {
		if seq != bucket.getSeq() {
			go node.doTask()
			return
		}
		header.insert(node)
	})
}

// run 开始执行定时扫描任务
func (t *TimeWheelTimer) run() {
	ticker := time.NewTicker(t.tickDuration)
	defer ticker.Stop()
	for {
		select {
		case cur := <-ticker.C:
			idx, _ := t.getIdxAndRounds(cur)
			t.process(idx)
		case <-t.shutDown.Done():
			return
		}
	}
}

// process 处理当前 bucket 待执行的任务，并且增加计数值
func (t *TimeWheelTimer) process(bucketIdx int) {
	bucket := t.timeWheel[bucketIdx]
	bucket.LockFunc(func(header *wheelNode) {
		bucket.setSeq(t.incrSeq())
		header.rangeNode(maxRangeCount, do)
	})
}

func (t *TimeWheelTimer) getIdxAndRounds(cur time.Time) (int, int64) {
	sub := int(cur.Sub(t.begin) / t.tickDuration)
	idx := sub % len(t.timeWheel)
	rounds := sub / len(t.timeWheel)
	if idx > 0 {
		rounds++
	}
	return idx, int64(rounds)
}

func do(bucket *wheelNode) (remove bool) {
	if atomic.AddInt64(&bucket.remainingRounds, -1) > 0 {
		return false
	}
	// 可以执行
	go bucket.doTask()
	return true
}

func (t *TimeWheelTimer) Stop() {
	t.shutDown.CancelFunc()
}

func (bucket *wheelBucket) LockFunc(f func(bucket *wheelNode)) {
	bucket.lock.Lock()
	defer bucket.lock.Unlock()
	f(bucket.wheelNode)
}

func newNode(remainingRounds int64, task func()) *wheelNode {
	item := &wheelNode{
		remainingRounds: remainingRounds,
	}
	item.task.Store(task)
	return item
}

func (w *wheelNode) cancel() {
	w.updateTask(cancelTask)
}
func (w *wheelNode) doTask() {
	w.task.Load().(func())()
}
func (w *wheelNode) updateTask(task func()) {
	w.task.Store(task)
}
func (w *wheelNode) rangeNode(maxCount int, f func(bucket *wheelNode) (remove bool)) {
	cur := w.nextNode()
	for i := 0; i < maxCount; i++ {
		if cur == nil {
			return
		}
		cur = func() *wheelNode {
			if f(cur) {
				return cur.remove()
			} else {
				return cur.nextNode()
			}
		}()

	}
}
func (w *wheelNode) nextNode() *wheelNode {
	if w == nil {
		return nil
	}
	return w.next
}

func (w *wheelNode) insert(node *wheelNode) {
	if w == nil {
		panic("append ori item is nil")
	}
	if node == nil {
		return
	}
	next := w.next
	w.next = node
	node.prev = w
	node.next = next
	if next != nil {
		next.prev = node
	}
}

func (w *wheelNode) remove() *wheelNode {
	if w == nil {
		return nil
	}
	if w.prev == nil {
		panic("can't remove header")
	}
	prev := w.prev
	next := w.next
	prev.next = next
	if next != nil {
		next.prev = prev
	}

	w.prev = nil
	w.next = nil
	return next
}
