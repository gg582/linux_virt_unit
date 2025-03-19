package lvirt_applicationUnit

import (
	"container/heap"
	"fmt"
)
type UserInfo struct {
    Username     string `json:"username"`
    UsernameIV   string `json:"username_iv"`
    Password     string `json:"password"`
    PasswordIV   string `json:"password_iv"`
    Key          string `json:"key"`
}

type ContainerInfo struct {
    Username string `json:"username"`
    UsernameIV string `json:"username_iv"`
    Password string `json:"password"`
    PasswordIV       string `json:"password_iv"`
    Key      string `json:"key"`
    TAG      string `json:"tag"`
    Serverip string `json:"serverip"`
    Serverport string `json:"serverport"`
    VMStatus     string `json:"vmstatus"`
}

var INFO ContainerInfo


// Int64Heap은 int64 값을 저장하는 최소 힙입니다.
type Int64Heap []int64

// Len은 요소 개수를 반환합니다.
func (h Int64Heap) Len() int { return len(h) }

// Less는 작은 값이 먼저 나오도록 정렬합니다.
func (h Int64Heap) Less(i, j int) bool { return h[i] < h[j] }

// Swap은 두 요소를 교환합니다.
func (h Int64Heap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Push는 새로운 요소를 추가합니다.
func (h *Int64Heap) Push(x interface{}) {
	*h = append(*h, x.(int64))
}

// Pop은 최솟값을 제거하고 반환합니다.
func (h *Int64Heap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

func main() {
	h := &Int64Heap{}
	heap.Init(h)

	// 값 추가
	heap.Push(h, int64(10))
	heap.Push(h, int64(3))
	heap.Push(h, int64(15))
	heap.Push(h, int64(7))
	heap.Push(h, int64(1))

	fmt.Println("Min Heap 순서대로 꺼내기:")
	for h.Len() > 0 {
		fmt.Println(heap.Pop(h)) // 작은 값부터 출력됨
	}
}

