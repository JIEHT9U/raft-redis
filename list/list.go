package list

//Create return new LinkedList
func Create() *LinkedList {
	return &LinkedList{}
}

//LinkedList type
type LinkedList struct {
	head, tail *Node
	length     int64
}

//Node type
type Node struct {
	Value      []byte
	hash       string
	prev, next *Node
}

//Next func iterate which linkedList
func (l *LinkedList) Next() chan *Node {

	transmitValue := make(chan *Node, 0)
	go func() {
		for n := l.head; n != nil; n = n.next {
			transmitValue <- n
		}
		close(transmitValue)
	}()
	return transmitValue

}

//Previous func iterate which reverse linkedList
func (l *LinkedList) Previous() chan *Node {

	transmitValue := make(chan *Node, 0)
	go func() {
		for n := l.tail; n != nil; n = n.prev {
			transmitValue <- n
		}
		close(transmitValue)
	}()
	return transmitValue

}

//CopyTo func
func (l *LinkedList) CopyTo(slice map[int64]interface{}, index int64) map[int64]interface{} {

	for n := range l.Next() {
		slice[index] = n.Value
		index++
	}
	return slice

}

//Count return len LinkedList
func (l *LinkedList) Count() int64 {
	return l.length
}

//Contains return bool
func (l *LinkedList) Contains(hash string) bool {
	if l.head == nil {
		return false
	}

	for node := range l.Next() {
		if node.hash == hash {
			return true
		}
	}

	return false
}

//RemoveLast func
func (l *LinkedList) RemoveLast() bool {
	if l.head == nil {
		return false
	}

	l.tail = l.tail.prev
	l.tail.next = nil
	l.length--
	return true
}

//RemoveFirst func
func (l *LinkedList) RemoveFirst() bool {
	if l.head == nil {
		return false
	}

	l.head = l.head.next
	l.head.prev = nil
	l.length--
	return true
}

//Remove func
func (l *LinkedList) Remove(hash string) bool {
	if l.head == nil {
		return false
	}

	for n := range l.Next() {
		if n.hash == hash {

			prev := n.prev
			next := n.next

			if prev != nil {
				prev.next = next
			} else {
				l.head = next
			}

			if next != nil {
				n.prev = prev
			} else {
				l.tail = prev
			}

			l.length--
			return true
		}
	}

	return false
}

//Clear func
func (l *LinkedList) Clear() {
	l.head = nil
	l.tail = nil
	l.length = 0
}

//AddLast func
func (l *LinkedList) AddLast(hash string, value []byte) *LinkedList {
	newNode := &Node{Value: value, hash: hash}
	if l.head == nil {
		l.head = newNode
	} else {
		l.tail.next = newNode
		newNode.prev = l.tail
	}
	l.tail = newNode
	l.length++
	return l
}

//AddFirst func
func (l *LinkedList) AddFirst(hash string, value []byte) *LinkedList {
	newNode := &Node{Value: value, hash: hash}
	if l.head == nil {
		l.tail = newNode
	} else {
		tmp := l.head
		newNode.next = tmp
		tmp.prev = newNode
	}
	l.head = newNode
	l.length++
	return l
}

//Add func
func (l *LinkedList) Add(hash string, value []byte) {
	l.AddLast(hash, value)
}
