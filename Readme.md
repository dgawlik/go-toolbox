
# Check

Fast checksum calculator.


### What it is

It's golang program that reads from standard input list of files separated by '\n' and concurrently computes their checksums. The output
format is

```
...
1efc4cc737a915cb /home/domg/c-samples/strndup.c
42cb73977c79e453 /home/domg/c-samples/strndup.o
84a7ffd82cdbe785 /home/domg/go/.git/FETCH_HEAD
dfaca45f2fc3bd7d /home/domgProjekty/go/.git/HEAD
b2f57ed69f0891f9 /home/domg/go/.git/config
351121088bf991bc /home/domg/go/.git/description
...
```
Performance is relatively fast...

### How to use
. . . 

* **Prepare the files list**

Download https://github.com/sharkdp/fd for your distribution.

Then 

* to list all files in directory

```
fdfind . /path/to/dir --type=f > file
```
* to list all files in directory including hidden files

```
fdfind . /path/to/dir -H --type=f > file
```

* to follow symbolic links

```
fdfind . /path/to/dir -H -L --type=f > file
```

* include only files with pattern

```
fdfind *.js /path/to/dir -H -L --type=f > file
```

* exclude directories files from search

```
add .gitignore to root directory
```

* **compute hashes**

Given you have file *snapshot* prepared

* fast compute

```
cat snapshot | ./check > sums.txt
```

* sha256

```
cat snapshot | ./check --sha256 > sums.txt
```

* human readable checksums

```
cat snapshot | ./check --colon > sums.txt
```


* **use diff to find differences**

```
diff prev_snapshot.txt curr_snapshot.txt
```

### Speed

Modern CPU, ~ 10K lines.


Wyhash:
```
domg@asus:~/Projekty/recursive-checksum$ time cat toscan | ./check  > 1.txt

real    0m10,098s
user    0m2,686s
sys     0m10,797s
```

Sha256
```
domg@asus:~/Projekty/recursive-checksum$ time cat toscan | ./check --sha256  > 1.txt

real    0m17,705s
user    0m33,343s
sys     0m7,498s
```