package main

import (
	"fmt"
	"time"

	"taskpool/pool"
)

func task(i int) {
	fmt.Printf("task :%d\n", i)
	time.Sleep(1 * time.Second)
}
func main() {
	fmt.Println("vim-go")
	taskPool, err := pool.BuildPool(pool.Size(5),
		pool.MaxWaitTaskNum(10), pool.ExpreWorkerCleanInterval(10),
		pool.PanicHandler(func(i interface{}) {
			fmt.Printf("execute error:%v", i)
		}))

	if err != nil {
		fmt.Printf("init error:%v", err)
		panic(err)
	}
	go func() {
		for {
			fmt.Println("waiting", taskPool.Waiting())
			time.Sleep(10 * time.Second)
		}

	}()
	for i := 0; i < 10; i++ {
		a := i
		fn := func() {
			time.Sleep(3 * time.Second)
			fmt.Printf("run task :%d\n", a)
		}
		go func() {
			if err := taskPool.Submit(fn); err != nil {
				fmt.Println(i, err)
			}
		}()

	}
	time.Sleep(1 * time.Second)
	taskPool.Exit()
	for i := 10; i < 13; i++ {
		a := i
		fn := func() {
			time.Sleep(3 * time.Second)
			fmt.Printf("run task :%d\n", a)
		}
		go func() {
			if err := taskPool.Submit(fn); err != nil {
				fmt.Println(a, err)
			}
		}()

	}
	time.Sleep(10 * time.Second)
}
