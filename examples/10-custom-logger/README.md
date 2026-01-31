# Custom logger 




If you implement this interface:
```go
package logger

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

```

You can use whatever logging tool you want, ensure to pass your custom logger to your runner:


```go
	r := runner.NewAdaptiveRunner(1, 20, 16000, // could be runtime.NumCPU()
		runner.WithAdaptiveLogger(/*your custom logger*/),
```


