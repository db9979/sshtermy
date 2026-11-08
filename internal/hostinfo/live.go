package hostinfo

// Live-Kennzahlen für die F6-Graphen: Ein kleines sh-Skript läuft, solange
// das Panel offen ist, in einer eigenen Exec-Sitzung und schreibt alle zwei
// Sekunden die Rohzähler aus /proc (nur lesend, nur Linux). Die Raten
// rechnet sshTermy selbst aus der Differenz zweier Proben.

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// LiveEvery: Abstand der Proben.
const LiveEvery = 2 * time.Second

// LiveScript: eine Probe je Block, Block endet mit "e". Ohne /proc/stat
// (BSD, macOS) kommt nur "nolive".
const LiveScript = `LC_ALL=C; export LC_ALL
[ -r /proc/stat ] || { echo nolive; exit 0; }
prev=""
while :; do
  head -n 1 /proc/stat | sed 's/^/c /'
  awk '/^MemTotal:|^MemAvailable:/{printf "m %s %s\n",$1,$2}' /proc/meminfo
  awk 'NR>2{sub(/^ */,"");sub(/:/," ");if($1!="lo"){r+=$2;t+=$10}}END{printf "n %.0f %.0f\n",r,t}' /proc/net/dev
  awk '$3~/^(sd[a-z]+|vd[a-z]+|xvd[a-z]+|hd[a-z]+|nvme[0-9]+n[0-9]+|mmcblk[0-9]+)$/{r+=$6;w+=$10}END{printf "d %.0f %.0f\n",r,w}' /proc/diskstats 2>/dev/null
  cur=$(cat /proc/[0-9]*/stat 2>/dev/null | awk '{i=index($0,"(");j=0;for(k=length($0);k>i;k--)if(substr($0,k,1)==")"){j=k;break};if(!i||!j)next;n=substr($0,i+1,j-i-1);split(substr($0,j+2),f," ");print $1, f[12]+f[13], n}')
  if [ -n "$prev" ]; then
    printf '%s\n--\n%s\n' "$prev" "$cur" | awk 's==0&&$0=="--"{s=1;next}s==0{p[$1]=$2;next}($1 in p){d=$2-p[$1];if(d>0){$1="";$2="";sub(/^  /,"");a[$0]+=d}}END{for(k in a)print a[k], k}' | sort -rn | head -n 5 | sed 's/^/p /'
  fi
  prev=$cur
  echo e
  sleep 2
done
`

// LiveProc: Prozess mit CPU-Zeit seit der letzten Probe (Ticks).
type LiveProc struct {
	Name  string
	Ticks uint64
}

// LiveSample: Rohzähler einer Probe.
type LiveSample struct {
	At                 time.Time
	CPUBusy, CPUTotal  uint64 // Ticks aller Kerne seit dem Start
	MemTotal, MemAvail uint64 // kB
	NetRx, NetTx       uint64 // Bytes seit dem Start (ohne lo)
	DiskR, DiskW       uint64 // Sektoren à 512 Bytes
	HasDisk            bool
	Procs              []LiveProc
}

// LiveParser setzt Zeilen zu Proben zusammen.
type LiveParser struct {
	cur LiveSample
	// NoLive: Gegenstelle hat kein /proc (kein Linux).
	NoLive bool
}

// Line nimmt eine Zeile; am Blockende kommt die fertige Probe zurück.
func (p *LiveParser) Line(l string) (LiveSample, bool) {
	f := strings.Fields(l)
	if len(f) == 0 {
		return LiveSample{}, false
	}
	u := func(i int) uint64 {
		if i >= len(f) {
			return 0
		}
		v, _ := strconv.ParseUint(f[i], 10, 64)
		return v
	}
	switch f[0] {
	case "nolive":
		p.NoLive = true
	case "c":
		// c cpu user nice system idle iowait irq softirq steal …
		var total, idle uint64
		for i := 2; i < len(f) && i < 10; i++ {
			v := u(i)
			total += v
			if i == 5 || i == 6 { // idle + iowait
				idle += v
			}
		}
		p.cur.CPUTotal, p.cur.CPUBusy = total, total-idle
	case "m":
		switch f[1] {
		case "MemTotal:":
			p.cur.MemTotal = u(2)
		case "MemAvailable:":
			p.cur.MemAvail = u(2)
		}
	case "n":
		p.cur.NetRx, p.cur.NetTx = u(1), u(2)
	case "d":
		p.cur.DiskR, p.cur.DiskW, p.cur.HasDisk = u(1), u(2), true
	case "p":
		if len(f) >= 3 {
			p.cur.Procs = append(p.cur.Procs, LiveProc{Name: strings.Join(f[2:], " "), Ticks: u(1)})
		}
	case "e":
		s := p.cur
		s.At = time.Now()
		p.cur = LiveSample{}
		return s, s.CPUTotal > 0
	}
	return LiveSample{}, false
}

// LivePoint: Raten zwischen zwei Proben.
type LivePoint struct {
	At       time.Time
	CPU      float64 // 0 … 1
	Mem      float64 // 0 … 1
	Net      float64 // Bytes/s (rein + raus)
	NetRx    float64
	NetTx    float64
	Disk     float64 // Bytes/s (lesen + schreiben)
	HasDisk  bool
	TopProcs []LiveProcPct
}

// LiveProcPct: Anteil eines Prozesses an der ganzen Maschine (0 … 1).
type LiveProcPct struct {
	Name string
	Pct  float64
}

// Rates rechnet aus zwei aufeinanderfolgenden Proben die Raten.
func Rates(a, b LiveSample) (LivePoint, bool) {
	dt := b.At.Sub(a.At).Seconds()
	if dt <= 0 || b.CPUTotal <= a.CPUTotal {
		return LivePoint{}, false
	}
	tot := float64(b.CPUTotal - a.CPUTotal)
	p := LivePoint{At: b.At, HasDisk: b.HasDisk}
	if b.CPUBusy >= a.CPUBusy {
		p.CPU = clamp01(float64(b.CPUBusy-a.CPUBusy) / tot)
	}
	if b.MemTotal > 0 {
		p.Mem = clamp01(1 - float64(b.MemAvail)/float64(b.MemTotal))
	}
	d := func(x, y uint64) float64 {
		if y < x {
			return 0 // Zähler übergelaufen oder Schnittstelle neu
		}
		return float64(y-x) / dt
	}
	p.NetRx, p.NetTx = d(a.NetRx, b.NetRx), d(a.NetTx, b.NetTx)
	p.Net = p.NetRx + p.NetTx
	p.Disk = (d(a.DiskR, b.DiskR) + d(a.DiskW, b.DiskW)) * 512
	for _, pr := range b.Procs {
		p.TopProcs = append(p.TopProcs, LiveProcPct{Name: pr.Name, Pct: clamp01(float64(pr.Ticks) / tot)})
	}
	sort.SliceStable(p.TopProcs, func(i, j int) bool { return p.TopProcs[i].Pct > p.TopProcs[j].Pct })
	return p, true
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Rate: Bytes/s lesbar (B/s, kB/s, MB/s, GB/s).
func Rate(v float64) string {
	switch {
	case v >= 1e9:
		return strconv.FormatFloat(v/1e9, 'f', 1, 64) + " GB/s"
	case v >= 1e6:
		return strconv.FormatFloat(v/1e6, 'f', 1, 64) + " MB/s"
	case v >= 1e3:
		return strconv.FormatFloat(v/1e3, 'f', 0, 64) + " kB/s"
	}
	return strconv.FormatFloat(v, 'f', 0, 64) + " B/s"
}
