# 🔤 Go Syntax Quick Reference

A compact reference for Go syntax patterns, shorthands, and idioms found in this codebase.

> Part of the [Go for Svelte Devs](GO_FOR_SVELTE_DEVS.md) guide. See [Documentation Index](INDEX.md).

---

## Variable Declarations

### Short Declaration (Inside Functions Only)

```go
// Type inferred from right side
x := 5                          // int
name := "tomoshibi"                // string
items := []string{"a", "b"}     // slice of strings
m := map[string]int{}           // empty map

// Multiple variables
a, b := 1, 2
url, err := parseURL(input)
```

### Explicit Declaration

```go
var x int                       // Zero value (0)
var name string = "tomoshibi"      // With initial value
var Log *slog.Logger            // Package-level (can't use :=)

// Multiple at once
var (
    a int
    b string
    c bool
)
```

### Zero Values

| Type             | Zero Value          |
| ---------------- | ------------------- |
| `int`, `float64` | `0`                 |
| `string`         | `""` (empty string) |
| `bool`           | `false`             |
| `*T` (pointer)   | `nil`               |
| `[]T` (slice)    | `nil`               |
| `map[K]V`        | `nil`               |
| `interface{}`    | `nil`               |
| `struct{}`       | All fields zero     |

---

## Functions

### Basic Function

```go
func add(a int, b int) int {
    return a + b
}

// Same-type params shorthand
func add(a, b int) int {
    return a + b
}
```

### Multiple Return Values

```go
func divide(a, b int) (int, error) {
    if b == 0 {
        return 0, fmt.Errorf("division by zero")
    }
    return a / b, nil
}

// Usage
result, err := divide(10, 2)
```

### Named Return Values

```go
func split(sum int) (x, y int) {
    x = sum * 4 / 9
    y = sum - x
    return  // Naked return (returns x, y)
}
```

### Variadic Functions

```go
func sum(nums ...int) int {
    total := 0
    for _, n := range nums {
        total += n
    }
    return total
}

// Usage
sum(1, 2, 3)
sum(nums...)  // Spread a slice
```

### Anonymous Functions / Closures

```go
// Assign to variable
handler := func(c *gin.Context) {
    c.JSON(200, gin.H{"ok": true})
}

// Immediately invoked (IIFE)
go func() {
    // runs in goroutine
}()

// With parameters
func(msg string) {
    fmt.Println(msg)
}("hello")
```

---

## Structs and Methods

### Struct Definition

```go
type User struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
    Age  int    `json:"age,omitempty"`
}

// Anonymous struct (one-off use)
config := struct {
    Host string
    Port int
}{"localhost", 8080}
```

### Creating Structs

```go
// All fields
u := User{ID: 1, Name: "Alice", Age: 30}

// Partial (others get zero values)
u := User{Name: "Bob"}

// Pointer to struct
u := &User{ID: 1, Name: "Alice"}

// new() returns pointer with zero values
u := new(User)  // *User with all zeros
```

### Methods (Receiver Functions)

```go
// Value receiver (gets copy)
func (u User) FullName() string {
    return u.Name
}

// Pointer receiver (can modify, preferred for large structs)
func (u *User) SetName(name string) {
    u.Name = name
}

// Call like this
user := &User{Name: "Alice"}
user.SetName("Bob")  // Go auto-dereferences
```

### Embedding (Composition)

```go
type Admin struct {
    User      // Embedded (anonymous field)
    Perms []string
}

admin := Admin{
    User:  User{Name: "Alice"},
    Perms: []string{"read", "write"},
}

admin.Name  // Accesses embedded User.Name
```

---

## Interfaces

### Definition

```go
type Scraper interface {
    Scrape(ctx context.Context, url string) (*Result, error)
}

// Multiple methods
type ReadWriter interface {
    Read(p []byte) (n int, err error)
    Write(p []byte) (n int, err error)
}

// Composing interfaces
type ReadWriteCloser interface {
    ReadWriter
    Close() error
}
```

### Empty Interface (Any Type)

```go
var x interface{}    // Can hold anything (like TypeScript's `any`)
x = 42
x = "hello"
x = User{}

// Go 1.18+ alias
var x any  // Same as interface{}
```

### Type Assertions

```go
var x interface{} = "hello"

s := x.(string)           // Panic if not string
s, ok := x.(string)       // Safe: ok is false if wrong type

// Type switch
switch v := x.(type) {
case string:
    fmt.Println("string:", v)
case int:
    fmt.Println("int:", v)
default:
    fmt.Println("unknown")
}
```

---

## Control Flow

### If Statements

```go
// Basic
if x > 0 {
    // ...
}

// With init statement (scoped to if block)
if err := doSomething(); err != nil {
    return err
}

// If-else-if
if x < 0 {
    // ...
} else if x > 100 {
    // ...
} else {
    // ...
}
```

### Switch

```go
// No break needed (implicit)
switch mode {
case "static":
    // ...
case "dynamic":
    // ...
default:
    // ...
}

// Multiple values
switch day {
case "Sat", "Sun":
    fmt.Println("Weekend")
}

// Expression-less (like if-else chain)
switch {
case x < 0:
    // ...
case x > 100:
    // ...
}

// fallthrough (explicit continue to next case)
switch n {
case 1:
    fmt.Println("one")
    fallthrough
case 2:
    fmt.Println("one or two")
}
```

### For Loop (The Only Loop)

```go
// Traditional
for i := 0; i < 10; i++ {
    // ...
}

// While-style
for x > 0 {
    x--
}

// Infinite
for {
    // break to exit
}

// Range over slice
for i, item := range items {
    fmt.Println(i, item)
}

// Range over map
for key, value := range myMap {
    fmt.Println(key, value)
}

// Ignore index
for _, item := range items {
    // ...
}

// Index only
for i := range items {
    // ...
}
```

### Defer, Panic, Recover

```go
// Defer: runs when function exits
func read() {
    f := openFile()
    defer f.Close()  // Guaranteed cleanup
    // ... work with f
}

// Multiple defers run in LIFO order
defer fmt.Println("1")  // Runs third
defer fmt.Println("2")  // Runs second
defer fmt.Println("3")  // Runs first

// Panic: like throwing an error
panic("something terrible happened")

// Recover: catch panics
defer func() {
    if r := recover(); r != nil {
        fmt.Println("Recovered:", r)
    }
}()
```

---

## Slices and Maps

### Slices

```go
// Create
s := []int{1, 2, 3}
s := make([]int, 5)       // length 5, zeros
s := make([]int, 0, 10)   // length 0, capacity 10

// Operations
len(s)                    // length
cap(s)                    // capacity
s = append(s, 4)          // append (returns new slice!)
s = append(s, 5, 6, 7)    // append multiple

// Slicing
s[1:3]    // elements 1, 2 (exclusive end)
s[:3]     // first 3
s[2:]     // from index 2 to end
s[:]      // copy whole slice

// Copy
dst := make([]int, len(src))
copy(dst, src)

// nil slice is valid
var s []int  // nil, len=0, cap=0
s == nil     // true
len(s)       // 0 (safe)
```

### Maps

```go
// Create
m := map[string]int{}
m := make(map[string]int)
m := map[string]int{"a": 1, "b": 2}

// Operations
m["key"] = 42             // set
val := m["key"]           // get (returns zero if missing)
val, ok := m["key"]       // check existence
delete(m, "key")          // delete

// Iterate (random order!)
for k, v := range m {
    fmt.Println(k, v)
}

// nil map is readable but not writable
var m map[string]int
_ = m["x"]  // OK (returns 0)
m["x"] = 1  // PANIC!
```

---

## Pointers

### Basics

```go
x := 42
p := &x     // p is pointer to x
*p = 43     // dereference: set x to 43
fmt.Println(*p)  // dereference: read x (43)

// nil pointer
var p *int  // nil
if p != nil {
    fmt.Println(*p)
}
```

### When to Use

```go
// Return pointer (caller can modify)
func NewUser() *User {
    return &User{}
}

// Accept pointer (to modify or avoid copy)
func (u *User) SetName(n string) {
    u.Name = n
}

// Accept value (small, immutable)
func (u User) GetName() string {
    return u.Name
}
```

### Pointer Shorthand

```go
// Go auto-dereferences for field access
u := &User{Name: "Alice"}
u.Name       // Same as (*u).Name

// But for method calls, either works
u.GetName()  // Works on *User or User
```

---

## Error Handling

### Creating Errors

```go
import "errors"
import "fmt"

// Simple error
err := errors.New("something went wrong")

// Formatted error
err := fmt.Errorf("failed to parse %s: %w", input, originalErr)
//                                     ↑ %w wraps original error

// Custom error type
type NotFoundError struct {
    ID string
}

func (e NotFoundError) Error() string {
    return fmt.Sprintf("not found: %s", e.ID)
}
```

### Handling Errors

```go
result, err := doSomething()
if err != nil {
    return err  // Propagate up
}

// Or handle specifically
if err != nil {
    log.Printf("Warning: %v", err)
    // continue anyway
}

// Check error type
var notFound NotFoundError
if errors.As(err, &notFound) {
    // Handle not found
}

// Check sentinel error
if errors.Is(err, io.EOF) {
    // Handle EOF
}
```

---

## Concurrency

### Goroutines

```go
// Start goroutine
go doWork()

// With closure
go func() {
    // ...
}()

// With parameters (capture by value)
for i := 0; i < 10; i++ {
    go func(n int) {  // Pass i as n
        fmt.Println(n)
    }(i)
}
```

### Channels

```go
// Create
ch := make(chan string)       // Unbuffered
ch := make(chan string, 10)   // Buffered (capacity 10)

// Send and receive
ch <- "hello"    // Send (blocks if full)
msg := <-ch      // Receive (blocks if empty)

// Close
close(ch)

// Range over channel (until closed)
for msg := range ch {
    fmt.Println(msg)
}

// Select (like switch for channels)
select {
case msg := <-ch1:
    fmt.Println(msg)
case ch2 <- "hello":
    fmt.Println("sent")
case <-time.After(1 * time.Second):
    fmt.Println("timeout")
default:
    fmt.Println("no activity")
}
```

### Context

```go
import "context"

// Background (root context)
ctx := context.Background()

// With timeout
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()

// With cancel
ctx, cancel := context.WithCancel(ctx)
// Call cancel() when done

// Check if cancelled
select {
case <-ctx.Done():
    return ctx.Err()
default:
    // continue
}
```

---

## String Formatting

### Printf Verbs

| Verb   | Description              | Example                  |
| ------ | ------------------------ | ------------------------ |
| `%v`   | Default format           | `fmt.Printf("%v", user)` |
| `%+v`  | With field names         | `{Name:Alice Age:30}`    |
| `%#v`  | Go syntax                | `User{Name:"Alice"}`     |
| `%T`   | Type                     | `main.User`              |
| `%s`   | String                   | `"hello"`                |
| `%d`   | Integer                  | `42`                     |
| `%f`   | Float                    | `3.140000`               |
| `%.2f` | 2 decimal places         | `3.14`                   |
| `%t`   | Boolean                  | `true`                   |
| `%p`   | Pointer                  | `0xc0000b4000`           |
| `%w`   | Wrap error (Errorf only) |                          |

### Common Functions

```go
fmt.Println("hello")              // Print with newline
fmt.Printf("x = %d\n", x)         // Formatted print
s := fmt.Sprintf("x = %d", x)     // Return formatted string
err := fmt.Errorf("error: %w", e) // Return formatted error
```

---

## JSON

### Struct Tags

```go
type User struct {
    ID       int    `json:"id"`                // Rename to "id"
    Name     string `json:"name"`
    Password string `json:"-"`                 // Exclude from JSON
    Age      int    `json:"age,omitempty"`     // Omit if zero
    Role     string `json:"role,string"`       // Force string encoding
}
```

### Marshal (Go → JSON)

```go
import "encoding/json"

data, err := json.Marshal(user)         // Returns []byte
data, err := json.MarshalIndent(user, "", "  ")  // Pretty print
```

### Unmarshal (JSON → Go)

```go
var user User
err := json.Unmarshal(data, &user)  // Pass pointer!

// From io.Reader (like HTTP response body)
err := json.NewDecoder(resp.Body).Decode(&user)
```

---

## Testing

### Basic Test

```go
// In file_test.go
package mypackage

import "testing"

func TestAdd(t *testing.T) {
    result := Add(2, 3)
    if result != 5 {
        t.Errorf("Add(2, 3) = %d; want 5", result)
    }
}
```

### Table-Driven Tests

```go
func TestAdd(t *testing.T) {
    tests := []struct {
        name     string
        a, b     int
        expected int
    }{
        {"positive", 2, 3, 5},
        {"negative", -1, -1, -2},
        {"zero", 0, 0, 0},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := Add(tt.a, tt.b); got != tt.expected {
                t.Errorf("got %d, want %d", got, tt.expected)
            }
        })
    }
}
```

### Running Tests

```bash
go test ./...              # All packages
go test -v ./...           # Verbose
go test -run TestAdd ./... # Specific test
go test -cover ./...       # With coverage
```

---

## Common Imports

```go
import (
    // I/O
    "io"
    "bufio"
    "os"

    // Formatting
    "fmt"
    "log"

    // Strings
    "strings"
    "strconv"

    // Data
    "encoding/json"
    "bytes"

    // HTTP
    "net/http"
    "net/url"

    // Time
    "time"
    "context"

    // Crypto
    "crypto/tls"

    // Compression
    "compress/gzip"
)
```

---

## Idioms and Patterns

### Error Handling Pattern

```go
if err != nil {
    return nil, fmt.Errorf("context: %w", err)
}
```

### Constructor Pattern

```go
func NewService(deps ...Dep) *Service {
    return &Service{dep: deps[0]}
}
```

### Functional Options

```go
type Option func(*Server)

func WithPort(p int) Option {
    return func(s *Server) { s.port = p }
}

func NewServer(opts ...Option) *Server {
    s := &Server{port: 8080}  // defaults
    for _, opt := range opts {
        opt(s)
    }
    return s
}

// Usage
srv := NewServer(WithPort(9000))
```

### Interface Satisfaction Check

```go
// Compile-time check that CollyScraper implements Scraper
var _ domain.Scraper = (*CollyScraper)(nil)
```

---

## Quick Commands

```bash
go run main.go           # Run
go build                 # Build binary
go build -o app ./cmd/x  # Build specific
go test ./...            # Test all
go fmt ./...             # Format code
go vet ./...             # Static analysis
go mod tidy              # Clean dependencies
go mod download          # Download dependencies
go get pkg@version       # Add/update dependency
go doc fmt.Println       # View documentation
```
