## compile
```bash
go build -o ~/.krew/bin/kubectl-pods main.go
```

## use kubectl plugin
```bash
kubectl pods
```

## table format
https://github.com/olekukonko/tablewriter
```bash
 go get github.com/olekukonko/tablewriter 
```
table rendering effect
```bash
> kubectl pods   
+---------------------------+-----------+----------------+---------+
|           NAME            | NAMESPACE |       IP       | STATUS  |
+---------------------------+-----------+----------------+---------+
| k8splay1-6784b6cb56-r66hd | default   | 10.244.236.188 | Running |
| nettool                   | default   | 10.244.236.177 | Running |
| nettool2                  | default   | 10.244.73.82   | Running |
+---------------------------+-----------+----------------+---------+                                                      
```