# Reference outputs

These are real, small functional runs, not performance claims.

- `fio/fio-3.33.json`: Debian fio 3.33, `--name=fixture --directory=/tmp/malta-fixture --size=1m --rw=randrw --rwmixread=70 --bs=4k --ioengine=psync --numjobs=1 --iodepth=1 --runtime=1 --time_based=1 --output-format=json+`.
- `mdtest/mdtest-4.0.0.txt`: official hpc/ior tag 4.0.0 (`967a9f65109760db8a3ac14a7fdd007f337d2960`), `mdtest -d /tmp/malta-fixture -n 8 -i 1 -F`.

Captured in an isolated Linux development container on 2026-10-26. First supported adapters are fio 3.33 JSON+ and mdtest 4.0.0 single-iteration rate summaries. Tool versions and raw evidence remain attached to every imported result. New versions require fixture validation before being declared supported.
