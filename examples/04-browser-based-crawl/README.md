
# Running:


This example uses:

- Memory Based Queue
- Memory Based Storage
- Standard Logger (StdLogger)
- __Chromium__ Crawler
- Basic Async Runner 


Then it:
1. Crawls all URL's in the queue (only one for now) 
2. Get's the byte value of the HTML 
3. Stores this in the storage.



The end result of running this will be something like:

```
2026/01/31 13:26:17 [DEBUG] Worker 0: Processing https://example.com (active: 1)
2026/01/31 13:26:17 [INFO] Crawling: https://example.com
2026/01/31 13:26:18 [INFO] Rendered page
2026/01/31 13:26:18 [DEBUG] Worker 0: Queue empty
2026/01/31 13:26:18 [INFO] All workers finished
2026/01/31 13:26:18 [INFO]
Done!
2026/01/31 13:26:18 [INFO] Got html: ## HTML HERE ## 
2026/01/31 13:26:18 [INFO] Stats: map[completed:1 pending:0]


```


