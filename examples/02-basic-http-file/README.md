This basic example uses:


- FileBased Queue 
- FileBased Storage
- Standard Logger (StdLogger)
- SimpleHTTPCrawler
- Basic Async Runner 


Then it:
1. Crawls all URL's in the queue (only one for now) 
2. Get's the byte value of the HTML 
3. Stores this in the storage.



The end result of running this will be something like:

```
NewFileQueueWithOptions: start
NewFileQueueWithOptions: creating dir data/queue/pending
NewFileQueueWithOptions: creating queue struct
NewFileQueueWithOptions: loading visited
NewFileQueueWithOptions: done
2026/01/31 13:06:51 [DEBUG] Worker 0: Processing https://example.com (active: 1)
2026/01/31 13:06:51 [DEBUG] Crawling: https://example.com
2026/01/31 13:06:51 [INFO] Saved page
2026/01/31 13:06:51 [DEBUG] Worker 0: Queue empty
2026/01/31 13:06:52 [DEBUG] Worker 1: Queue empty
2026/01/31 13:06:52 [INFO] All workers finished

```

```bash
tree data
```

```
data
├── queue
│   ├── pending
│   └── visited.json
└── storage
    └── URLNAME.json

4 directories, 2 files
```


With "pending" being empty, "visited.json" being an array of visited URL's
And URLNAME.json containing one value, the HTML


