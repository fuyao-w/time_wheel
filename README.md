# 时间轮内存延迟消息

- 支持任意时间的延迟任务
- 可以自定义最小延迟时间粒度、时间轮长度


用法：
```go
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
```

