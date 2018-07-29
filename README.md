# GOLANG RAFT REDIS

The project is created as a sample code. Not intended for `production`.

GoRedis is written in Go and uses the `Raft consensus algorithm` to manage a highly-available replicated log.

Storage support several types of data Hash Tabe, Linked List and String with the ability to specify the expirationtime in seconds for each key.

### Capabilities

* provide highly available
* save data to disk
* TTL for each key 

### Running a local GoRedis cluster

```bash
docker-compose up
```

### Client Connection

```bash
telnet 127.0.0.1 3000
```

### Operators:

* SET key value

    Set key to hold the string value. If key already holds a value, it is overwritten, regardless of its type. Any previous time to live associated with the key is discarded on successful SET operation.

    Examples:
    
    ```bash
    [127.0.0.1:43786]> SET mykey "Hello"
    "OK"
    [127.0.0.1:43786]> GET mykey
    "Hello"
    [127.0.0.1:43786]> 
    ```
     
* GET key

    Get the value of key. If the key does not exist the special value nil is returned. An error is returned if the value stored at key is not a string, because GET only handles string values.

    Examples:
    
    ```bash
    [127.0.0.1:43796] > GET nonexisting
    (key not found)
    [127.0.0.1:43796] > SET mykey "Hello"
    "OK"
    [127.0.0.1:43796] > GET mykey
    "Hello"
    [127.0.0.1:43796] >
    ```

* EXPIRE key seconds

    Set a timeout on key. After the timeout has expired, the key will automatically be deleted. A key with an associated timeout is often said to be volatile in Redis terminology.

    Examples:
    
    ```bash
    [127.0.0.1:43796] > SET mykey "Hello"
    "OK"
    [127.0.0.1:43796] > EXPIRE mykey 10
    "OK"
    [127.0.0.1:43796] > TTL mykey
    TTL 354.793302ms
    [127.0.0.1:43796] > SET mykey "Hello World"
    "OK"
    [127.0.0.1:43796] > TTL mykey
    TTL infinity
    [127.0.0.1:43796] > 
    ```

* DEL key

    Removes the specified key. A key is ignored if it does not exist.

    Examples:
    
    ```bash
    [127.0.0.1:43796] > SET key1 "Hello"
    "OK"
    [127.0.0.1:43796] > DEL key1
    "OK"
    [127.0.0.1:43796] > 
    ```

* LPUSH key value [value ...]

    Insert all the specified values at the head of the list stored at key. If key does not exist, it is created as empty list before performing the push operations. When key holds a value that is not a list, an error is returned.

    It is possible to push multiple elements using a single command call just specifying multiple arguments at the end of the command. Elements are inserted one after the other to the head of the list, from the leftmost element to the rightmost element. So for instance the command LPUSH mylist a b c will result into a list containing c as first element, b as second element and a as third element.

    Examples:
    
    ```bash
    [127.0.0.1:43796] > LPUSH mylist "world"
    "OK"
    [127.0.0.1:43796] > LPUSH mylist "hello"
    "OK"
    [127.0.0.1:43796] > LRANGE mylist 0 -1
    1) "hello"
    2) "world"
    [127.0.0.1:43796] > 
    ```

* RPUSH key value [value ...]

    Insert all the specified values at the tail of the list stored at key. If key does not exist, it is created as empty list before performing the push operation. When key holds a value that is not a list, an error is returned.

    It is possible to push multiple elements using a single command call just specifying multiple arguments at the end of the command. Elements are inserted one after the other to the tail of the list, from the leftmost element to the rightmost element. So for instance the command RPUSH mylist a b c will result into a list containing a as first element, b as second element and c as third element.

    Examples:
    
    ```bash
    [127.0.0.1:43796] > RPUSH mylist "hello"
    "OK"
    [127.0.0.1:43796] > RPUSH mylist "world"
    "OK"
    [127.0.0.1:43796] > LRANGE mylist 0 -1
    1) "hello"
    2) "world"
    [127.0.0.1:43796] > 
    ```

* LRANGE key start stop

    Returns the specified elements of the list stored at key. The offsets start and stop are zero-based indexes, with 0 being the first element of the list (the head of the list), 1 being the next element and so on.

    Examples:
    
    ```bash
    [127.0.0.1:43796] > RPUSH mylist "one"
    "OK"
    [127.0.0.1:43796] > RPUSH mylist "two"
    "OK"
    [127.0.0.1:43796] > RPUSH mylist "three"
    "OK"
    [127.0.0.1:43796] > LRANGE mylist 0 0
    "mylist:one"
    [127.0.0.1:43796] > LRANGE mylist 0 -1
    "one"
    "two"
    "three"
    ```

* LLEN key

    Returns the length of the list stored at key.

    Examples:
    
    ```bash
    [127.0.0.1:43796] > LPUSH mylist "World"
    "OK"
    [127.0.0.1:43796] > LPUSH mylist "Hello"
    "OK"
    [127.0.0.1:43796] > LLEN mylist
    len:2
    [127.0.0.1:43796] > 
    ```

* HSET key field value

    Sets field in the hash stored at key to value. If key does not exist, a new key holding a hash is created. If field already exists in the hash, it is overwritten.

    Examples:
    
    ```bash
    [127.0.0.1:43796] > HSET myhash field1 "Hello"
    "OK"
    [127.0.0.1:43796] > HGET myhash field1
    "Hello"
    [127.0.0.1:43796] > 

    ```
* HGET key field

    Returns the value associated with field in the hash stored at key.

    Examples:
    
    ```bash
    [127.0.0.1:43796] > HSET myhash field1 "foo"
    "OK"
    [127.0.0.1:43796] > HGET myhash field1
    "foo"
    [127.0.0.1:43796] > HGET myhash field2
    (nil)
    [127.0.0.1:43796] > 
    ```

* HGETALL key

    Returns all fields and values of the hash stored at key. In the returned value, every field name is followed by its value, so the length of the reply is twice the size of the hash.

    Examples:
    
    ```bash
    [127.0.0.1:43796] > HSET myhash field1 "Hello"
    "OK"
    [127.0.0.1:43796] > HSET myhash field2 "World"
    "OK"
    [127.0.0.1:43796] > HGETALL myhash
    "field1:Hello"
    "field2:World"
    [127.0.0.1:43796] > 
    ```

    

    
