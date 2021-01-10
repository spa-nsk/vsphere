package vsphere

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
)

const initFloat64min float64 = 7777777777.0

var (
	infrastructureGlobal  *VMInfrastructure
	infrastructureMutex   sync.RWMutex
	vminfraNotInitialised string = "ошибка VMInfrastructure не инициализирован"
	cpuUsage                     = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vm_CPUUsageAverage_percent",
			Help: "VM CPU Perfonmance",
		},
		[]string{"Vcenter", "Datacenter", "Cluster", "Name"},
	)
	memUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vm_MEMUsageAverage_percent",
			Help: "VM Memory Perfonmance",
		},
		[]string{"Vcenter", "Datacenter", "Cluster", "Name"},
	)
	diskUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vm_DiskUsageAverage_kbit_sec",
			Help: "VM Disk Perfonmance",
		},
		[]string{"Vcenter", "Datacenter", "Cluster", "Name"},
	)
	diskReadUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vm_virtualDiskReadAverage_kbit_sec",
			Help: "VM Disk read Perfonmance",
		},
		[]string{"Vcenter", "Datacenter", "Cluster", "Name"},
	)
	diskWriteUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vm_virtualDiskWriteAverage_kbit_sec",
			Help: "VM Disk write Perfonmance",
		},
		[]string{"Vcenter", "Datacenter", "Cluster", "Name"},
	)
	diskReadLatency = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vm_virtualDiskTotalReadLatencyAverage_ms",
			Help: "VM Disk Read Latency Perfonmance",
		},
		[]string{"Vcenter", "Datacenter", "Cluster", "Name"},
	)
	diskWriteLatency = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vm_virtualDiskTotalWriteLatencyAverage_ms",
			Help: "VM Disk Write Latency Perfonmance",
		},
		[]string{"Vcenter", "Datacenter", "Cluster", "Name"},
	)
	netUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vm_NetUsageAverage_kbit_sec",
			Help: "VM Network Perfonmance",
		},
		[]string{"Vcenter", "Datacenter", "Cluster", "Name"},
	)
)

func init() {
	prometheus.MustRegister(cpuUsage)
	prometheus.MustRegister(memUsage)
	prometheus.MustRegister(diskUsage)
	prometheus.MustRegister(diskReadUsage)
	prometheus.MustRegister(diskWriteUsage)
	prometheus.MustRegister(diskReadLatency)
	prometheus.MustRegister(diskWriteLatency)
	prometheus.MustRegister(netUsage)
}

func setMetric() {
	if infrastructureGlobal == nil {
		return
	}
	infrastructureGlobal.VM.RLock()
	for _, vm := range infrastructureGlobal.VMList {
		ind := len(vm.Perf) - 1
		if ind >= 0 {
			cpuUsage.WithLabelValues(vm.Vcenter, vm.Datacenter, vm.Cluster, vm.VMName).Set(vm.Perf[ind].CPUUsage)
			memUsage.WithLabelValues(vm.Vcenter, vm.Datacenter, vm.Cluster, vm.VMName).Set(vm.Perf[ind].MEMUsage)
			diskUsage.WithLabelValues(vm.Vcenter, vm.Datacenter, vm.Cluster, vm.VMName).Set(vm.Perf[ind].DiskUsage)
			netUsage.WithLabelValues(vm.Vcenter, vm.Datacenter, vm.Cluster, vm.VMName).Set(vm.Perf[ind].NetUsage)
			diskReadLatency.WithLabelValues(vm.Vcenter, vm.Datacenter, vm.Cluster, vm.VMName).Set(vm.Perf[ind].DiskTotalReadLatency)
			diskWriteLatency.WithLabelValues(vm.Vcenter, vm.Datacenter, vm.Cluster, vm.VMName).Set(vm.Perf[ind].DiskTotalWriteLatency)
			diskWriteUsage.WithLabelValues(vm.Vcenter, vm.Datacenter, vm.Cluster, vm.VMName).Set(vm.Perf[ind].DiskWrite)
			diskReadUsage.WithLabelValues(vm.Vcenter, vm.Datacenter, vm.Cluster, vm.VMName).Set(vm.Perf[ind].DiskRead)
		}
	}
	infrastructureGlobal.VM.RUnlock()
}

func (vmp *VMInfo) importData(vm mo.VirtualMachine, infra *VMInfrastructure) {

	vmp.VMName = vm.Name
	vmp.VMGuestFullName = vm.Summary.Config.GuestFullName
	vmp.VMPathName = vm.Summary.Config.VmPathName

	if vm.Summary.Runtime.PowerState == "poweredOff" {
		vmp.VMPowerState = false
	} else {
		vmp.VMPowerState = true
	}

	vmp.VMUUID = vm.Summary.Config.Uuid
	vmp.VMGuestID = vm.Summary.Config.GuestId
	vmp.VMAnnotation = vm.Summary.Config.Annotation

	if vm.Config != nil {

		vmp.VMNumCores = vm.Config.Hardware.NumCPU * vm.Config.Hardware.NumCoresPerSocket
		vmp.VMNumCPU = vm.Config.Hardware.NumCPU
		vmp.VMMemorySizeGB = float64(vm.Config.Hardware.MemoryMB) / 1024.0
	} else {
		vmp.VMNumCores = vm.Summary.Config.NumCpu
		vmp.VMNumCPU = vm.Summary.Config.NumCpu
		vmp.VMMemorySizeGB = float64(vm.Summary.Config.MemorySizeMB) / 1024.0
	}

	vmp.VMCapacityGb = 0.0
	vmp.VMFreeSpaceGb = 0.0
	if vm.Storage != nil {
		if len(vm.Storage.PerDatastoreUsage) > 0 {
			for _, st := range vm.Storage.PerDatastoreUsage {
				usageDS := VMUsageOnDatastore{Datastore: vmp.Vcenter + "/" + st.Datastore.Value,
					CommittedGb:            float64(st.Committed) / (1024.0 * 1024.0 * 1024.0),
					UncommittedGb:          float64(st.Uncommitted) / (1024.0 * 1024.0 * 1024.0),
					UnsharedGb:             float64(st.Unshared) / (1024.0 * 1024.0 * 1024.0),
					PercentAllocateStorage: 0.0}
				if ds := infra.DSList[vmp.Vcenter+"/"+st.Datastore.Value]; ds != nil {
					usageDS.PercentAllocateStorage = 100.0 * usageDS.CommittedGb / ds.CapacityGb
					usageDS.StorageName = ds.StorageName
				} else {
					fmt.Println("Ошибка поиска хранилища", st.Datastore)
				}
				vmp.UsageDatastore = append(vmp.UsageDatastore, usageDS)
			}
		}
	}
	if vm.Guest != nil {
		vmp.VMHostName = vm.Guest.HostName

		vmp.VMIpAddress = vm.Guest.IpAddress
		vmp.VMNumVirtualDisks = int32(len(vm.Guest.Disk))

		for _, disk := range vm.Guest.Disk {
			d := GuestDiskInfo{DiskPath: disk.DiskPath}
			d.CapacityGb = float64(disk.Capacity) / (1024.0 * 1024.0 * 1024)
			d.FreeSpaceGb = float64(disk.FreeSpace) / (1024.0 * 1024.0 * 1024)
			d.PercentUsed = 100.0 * (d.CapacityGb - d.FreeSpaceGb) / d.CapacityGb
			vmp.VMCapacityGb = vmp.VMCapacityGb + d.CapacityGb
			vmp.VMFreeSpaceGb = vmp.VMFreeSpaceGb + d.FreeSpaceGb
			vmp.VMDisk = append(vmp.VMDisk, d)
		}

		if vmp.VMCapacityGb != 0.0 {
			vmp.VMPercentUsed = 100.0 * (vmp.VMCapacityGb - vmp.VMFreeSpaceGb) / vmp.VMCapacityGb
		}

	} else {
		fmt.Println(vmp.VMName, "vm.Guest == nil")
	}

	vmp.VMNumEthernetCards = vm.Summary.Config.NumEthernetCards
	if vmp.VMNumVirtualDisks == 0 {
		vmp.VMNumVirtualDisks = vm.Summary.Config.NumVirtualDisks
	}
	if vm.Summary.Storage != nil {
		vmp.VMDiskCommittedGB = float64(vm.Summary.Storage.Committed) / (1024.0 * 1024.0 * 1024)
		vmp.VMDiskUncommittedGB = float64(vm.Summary.Storage.Uncommitted) / (1024.0 * 1024.0 * 1024.0)
	}
}

func (vmp *VMInfo) vmClearPerfomance() {
	vmp.CPUUsageMaximum = 0.0
	vmp.CPUUsageMaximumTime = time.Time{}
	vmp.CPUUsageAverage = 0.0
	vmp.CPUUsageMinimum = initFloat64min
	vmp.CPUUsageMinimumTime = time.Time{}
	vmp.MEMUsageMaximum = 0.0
	vmp.MEMUsageMaximumTime = time.Time{}
	vmp.MEMUsageAverage = 0.0
	vmp.MEMUsageMinimum = initFloat64min
	vmp.MEMUsageMinimumTime = time.Time{}
	vmp.DiskUsageMaximum = 0.0
	vmp.DiskUsageMaximumTime = time.Time{}
	vmp.DiskUsageAverage = 0.0
	vmp.DiskUsageMinimum = initFloat64min
	vmp.DiskUsageMinimumTime = time.Time{}
	vmp.DiskTotalReadLatencyMaximum = 0.0
	vmp.DiskTotalReadLatencyMaximumTime = time.Time{}
	vmp.DiskTotalReadLatencyAverage = 0.0
	vmp.DiskTotalReadLatencyMinimum = initFloat64min
	vmp.DiskTotalReadLatencyMinimumTime = time.Time{}
	vmp.DiskTotalWriteLatencyMaximum = 0.0
	vmp.DiskTotalWriteLatencyMaximumTime = time.Time{}
	vmp.DiskTotalWriteLatencyAverage = 0.0
	vmp.DiskTotalWriteLatencyMinimum = initFloat64min
	vmp.DiskTotalWriteLatencyMinimumTime = time.Time{}
	vmp.DiskWriteMaximum = 0.0
	vmp.DiskWriteMaximumTime = time.Time{}
	vmp.DiskWriteAverage = 0.0
	vmp.DiskWriteMinimum = initFloat64min
	vmp.DiskWriteMinimumTime = time.Time{}
	vmp.DiskReadMaximum = 0.0
	vmp.DiskReadMaximumTime = time.Time{}
	vmp.DiskReadAverage = 0.0
	vmp.DiskReadMinimum = initFloat64min
	vmp.DiskReadMinimumTime = time.Time{}
	vmp.NetUsageMaximum = 0.0
	vmp.NetUsageMaximumTime = time.Time{}
	vmp.NetUsageAverage = 0.0
	vmp.NetUsageMinimum = initFloat64min
	vmp.NetUsageMinimumTime = time.Time{}
	vmp.PerfCount = 0
}

func (cl *Cluster) clClearPerfomance() {
	cl.CPUMhzAvg = 0
	cl.AllNumCPUPkgs = 0
	cl.AllNumCPUCores = 0
	cl.AllNumCPUThreads = 0
	cl.AllMemorySizeGB = 0.0
	cl.CPUUsageMaximum = 0.0
	cl.CPUUsageMaximumTime = time.Time{}
	cl.CPUUsageAverage = 0.0
	cl.CPUUsageMinimum = initFloat64min
	cl.CPUUsageMinimumTime = time.Time{}
	cl.MEMUsageMaximum = 0.0
	cl.MEMUsageMaximumTime = time.Time{}
	cl.MEMUsageAverage = 0.0
	cl.MEMUsageMinimum = initFloat64min
	cl.MEMUsageMinimumTime = time.Time{}
	cl.DiskUsageMaximum = 0.0
	cl.DiskUsageMaximumTime = time.Time{}
	cl.DiskUsageAverage = 0.0
	cl.DiskUsageMinimum = initFloat64min
	cl.DiskUsageMinimumTime = time.Time{}
	cl.DiskTotalReadLatencyMaximum = 0.0
	cl.DiskTotalReadLatencyMaximumTime = time.Time{}
	cl.DiskTotalReadLatencyAverage = 0.0
	cl.DiskTotalReadLatencyMinimum = initFloat64min
	cl.DiskTotalReadLatencyMinimumTime = time.Time{}
	cl.DiskTotalWriteLatencyMaximum = 0.0
	cl.DiskTotalWriteLatencyMaximumTime = time.Time{}
	cl.DiskTotalWriteLatencyAverage = 0.0
	cl.DiskTotalWriteLatencyMinimum = initFloat64min
	cl.DiskTotalWriteLatencyMinimumTime = time.Time{}
	cl.DiskWriteMaximum = 0.0
	cl.DiskWriteMaximumTime = time.Time{}
	cl.DiskWriteAverage = 0.0
	cl.DiskWriteMinimum = initFloat64min
	cl.DiskWriteMinimumTime = time.Time{}
	cl.DiskReadMaximum = 0.0
	cl.DiskReadMaximumTime = time.Time{}
	cl.DiskReadAverage = 0.0
	cl.DiskReadMinimum = initFloat64min
	cl.DiskReadMinimumTime = time.Time{}
	cl.NetUsageMaximum = 0.0
	cl.NetUsageMaximumTime = time.Time{}
	cl.NetUsageAverage = 0.0
	cl.NetUsageMinimum = initFloat64min
	cl.NetUsageMinimumTime = time.Time{}
	cl.CPUBilling = 0.0
	cl.DiskBilling = 0.0
	cl.MEMBilling = 0.0
	cl.NetBilling = 0.0
}

func (hs *HostInfo) hsClearPerfomance() {
	hs.CPUUsageMaximum = 0.0
	hs.CPUUsageMaximumTime = time.Time{}
	hs.CPUUsageAverage = 0.0
	hs.CPUUsageMinimum = initFloat64min
	hs.CPUUsageMinimumTime = time.Time{}
	hs.MEMUsageMaximum = 0.0
	hs.MEMUsageMaximumTime = time.Time{}
	hs.MEMUsageAverage = 0.0
	hs.MEMUsageMinimum = initFloat64min
	hs.MEMUsageMinimumTime = time.Time{}
	hs.DiskUsageMaximum = 0.0
	hs.DiskUsageMaximumTime = time.Time{}
	hs.DiskUsageAverage = 0.0
	hs.DiskUsageMinimum = initFloat64min
	hs.DiskUsageMinimumTime = time.Time{}
	hs.DiskTotalReadLatencyMaximum = 0.0
	hs.DiskTotalReadLatencyMaximumTime = time.Time{}
	hs.DiskTotalReadLatencyAverage = 0.0
	hs.DiskTotalReadLatencyMinimum = initFloat64min
	hs.DiskTotalReadLatencyMinimumTime = time.Time{}
	hs.DiskTotalWriteLatencyMaximum = 0.0
	hs.DiskTotalWriteLatencyMaximumTime = time.Time{}
	hs.DiskTotalWriteLatencyAverage = 0.0
	hs.DiskTotalWriteLatencyMinimum = initFloat64min
	hs.DiskTotalWriteLatencyMinimumTime = time.Time{}
	hs.DiskWriteMaximum = 0.0
	hs.DiskWriteMaximumTime = time.Time{}
	hs.DiskWriteAverage = 0.0
	hs.DiskWriteMinimum = initFloat64min
	hs.DiskWriteMinimumTime = time.Time{}
	hs.DiskReadMaximum = 0.0
	hs.DiskReadMaximumTime = time.Time{}
	hs.DiskReadAverage = 0.0
	hs.DiskReadMinimum = initFloat64min
	hs.DiskReadMinimumTime = time.Time{}
	hs.NetUsageMaximum = 0.0
	hs.NetUsageMaximumTime = time.Time{}
	hs.NetUsageAverage = 0.0
	hs.NetUsageMinimum = initFloat64min
	hs.NetUsageMinimumTime = time.Time{}
	hs.Perf = nil
	hs.PerfCount = 0
}

func (hs *HostInfo) importData(hi mo.HostSystem) {

	s := hi.Summary
	h := s.Hardware
	hs.MemorySizeGB = float64(h.MemorySize) / (1024.0 * 1024.0 * 1024.0)
	hs.HostMaxVirtualDiskCapacity = float64(hi.Runtime.HostMaxVirtualDiskCapacity) / (1024.0 * 1024.0)
	hs.Name = s.Config.Name

	hs.Vendor = h.Vendor
	hs.Model = h.Model
	hs.UUID = h.Uuid
	hs.CPUModel = h.CpuModel
	hs.CPUMhz = h.CpuMhz
	hs.NumCPUPkgs = h.NumCpuPkgs
	hs.NumCPUCores = h.NumCpuCores
	hs.NumCPUThreads = h.NumCpuThreads
	hs.NumNics = h.NumNics
	hs.NumHBAs = h.NumHBAs

	if hi.Runtime.PowerState == "poweredOff" {
		hs.PowerState = false
	} else {
		hs.PowerState = true
	}
	hs.InMaintenanceMode = hi.Runtime.InMaintenanceMode

	for _, info := range hi.Hardware.SystemInfo.OtherIdentifyingInfo {
		switch info.IdentifierType.GetElementDescription().Key {
		case "SerialNumberTag":
			hs.SerialNumber = info.IdentifierValue
		case "EnclosureSerialNumberTag":
			hs.EnclosureSerialNumber = info.IdentifierValue
		}
	}
	if hi.Hardware.SystemInfo.SerialNumber != "" {
		hs.SerialNumber = hi.Hardware.SystemInfo.SerialNumber
	}
}

//ByName сортировка по имени виртуальной машины
//
type ByName []mo.VirtualMachine

func (n ByName) Len() int           { return len(n) }
func (n ByName) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }
func (n ByName) Less(i, j int) bool { return n[i].Name < n[j].Name }

//VMListID сортировка по ID виртуальной машины
//
type VMListID []VMInfo

func (n VMListID) Len() int           { return len(n) }
func (n VMListID) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }
func (n VMListID) Less(i, j int) bool { return n[i].ID < n[j].ID }

//VMListCluster сортировка по именам кластеров
//
type VMListCluster []VMInfo

func (n VMListCluster) Len() int      { return len(n) }
func (n VMListCluster) Swap(i, j int) { n[i], n[j] = n[j], n[i] }
func (n VMListCluster) Less(i, j int) bool {
	if strings.Compare(n[i].Cluster, n[j].Cluster) < 0 {
		return true
	}
	return false
}

//VMListName сортировка по именам
//
type VMListName []VMInfo

func (n VMListName) Len() int      { return len(n) }
func (n VMListName) Swap(i, j int) { n[i], n[j] = n[j], n[i] }
func (n VMListName) Less(i, j int) bool {
	if strings.Compare(n[i].VMName, n[j].VMName) < 0 {
		return true
	}
	return false
}

//HSListID сортировка хостов по ID
//
type HSListID []HostInfo

func (n HSListID) Len() int           { return len(n) }
func (n HSListID) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }
func (n HSListID) Less(i, j int) bool { return n[i].ID < n[j].ID }

//HSListName сортировка хостов по именам
//
type HSListName []*HostInfo

func (n HSListName) Len() int           { return len(n) }
func (n HSListName) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }
func (n HSListName) Less(i, j int) bool { return n[i].Name < n[j].Name }

//ClusterCPUPerf сортировка кластеров по индексу производительности процессоров
//
type ClusterCPUPerf []*Cluster

func (n ClusterCPUPerf) Len() int      { return len(n) }
func (n ClusterCPUPerf) Swap(i, j int) { n[i], n[j] = n[j], n[i] }
func (n ClusterCPUPerf) Less(i, j int) bool {
	var summI, summJ float64
	for _, hs := range n[i].Hosts {
		summI = summI + float64(hs.CPUMhz)*float64(hs.NumCPUCores)
	}
	for _, hs := range n[j].Hosts {
		summJ = summJ + float64(hs.CPUMhz)*float64(hs.NumCPUCores)
	}
	return !(summI < summJ) //сортировка по убыванию
}

//ClusterDate сортировка кластеров по дате
//
type ClusterDate []*Cluster

func (n ClusterDate) Len() int      { return len(n) }
func (n ClusterDate) Swap(i, j int) { n[i], n[j] = n[j], n[i] }
func (n ClusterDate) Less(i, j int) bool {
	return (n[i].Date.Before(n[j].Date)) //сортировка сортировка по возрастанию
}

//ClearUsersAndPasswords очистка логинов и паролей управляющих vcenter
//
func (infra *VMInfrastructure) ClearUsersAndPasswords() {
	clearString := "********"
	infra.VC.Lock()
	for i := range infra.VCList {
		infra.VCList[i].Username = clearString
		infra.VCList[i].Password = clearString
	}
	infra.VC.Unlock()
}

func (infra *VMInfrastructure) importUsersAndPasswords(from *VMInfrastructure) {
	vcList := make([]VCenter, 0)
	from.VC.RLock()
	for _, rec := range from.VCList {
		vcList = append(vcList, VCenter{Hostname: rec.Hostname, Username: rec.Username, Password: rec.Password})
	}
	from.VC.RUnlock()
	infra.VC.Lock()
	infra.VCList = vcList
	infra.VC.Unlock()
}

//SetGlobalVMInfrastructure установить значение infrastructureGlobal
func SetGlobalVMInfrastructure(init *VMInfrastructure) *VMInfrastructure {

	infrastructureMutex.RLock()
	infra := infrastructureGlobal
	infrastructureMutex.RUnlock()

	init.importUsersAndPasswords(infra)
	//заблокировать операции с infrastructureGlobal
	infrastructureMutex.Lock()

	infrastructureGlobal = init
	infrastructureGlobal.IN.Lock()
	infrastructureGlobal.Initialized = true
	infrastructureGlobal.IN.Unlock()
	infrastructureMutex.Unlock()
	if infra != nil {
		//обработка дополнения метрик
	}
	//снятие блокировки
	infrastructureMutex.RLock()
	defer infrastructureMutex.RUnlock()
	return infrastructureGlobal
}

//GlobalInitVMInfrastructure инициализация внктренних структур
func GlobalInitVMInfrastructure() *VMInfrastructure {

	infrastructureMutex.Lock()
	defer infrastructureMutex.Unlock()
	infrastructureGlobal = &VMInfrastructure{}
	infrastructureGlobal.Init()
	infrastructureGlobal.IN.Lock()
	infrastructureGlobal.Initialized = false
	infrastructureGlobal.IN.Unlock()
	return infrastructureGlobal
}

//GetGlobalVMInfrastructure получить указатель на объект инфраструктуры
func GetGlobalVMInfrastructure() *VMInfrastructure {
	infrastructureMutex.Lock()
	defer infrastructureMutex.Unlock()

	if infrastructureGlobal == nil {
		infrastructureGlobal = &VMInfrastructure{}
	}
	return infrastructureGlobal
}

//DestroyGlobalVMInfrastructureObj разрушить объекты взвимосвязанные инфраструктуры
func DestroyGlobalVMInfrastructureObj() {
	infrastructureMutex.Lock()
	defer infrastructureMutex.Unlock()
	if infrastructureGlobal != nil {
		infrastructureGlobal.Destroy()
		//           infrastructureGlobal = nil
	}
}

//GetName получить имя управляющего центра
func (vcenter *VCenter) GetName() string {
	return vcenter.Hostname
}

//getCluster получить указатель на структуру кластера по его имени
func (infra *VMInfrastructure) getCluster(name string) (*Cluster, error) {
	for i := range infra.CLList {
		if name == infra.CLList[i].Name {
			return infra.CLList[i], nil
		}
	}
	return &Cluster{}, errors.New("Cluster: " + name + " not found")
}

//GetHostList получить срез с данными всех хостов по всем зарегистрированным vcenter
func (infra *VMInfrastructure) GetHostList() ([]HostInfo, error) {

	if infra == nil {
		return nil, errors.New(vminfraNotInitialised)
	}

	var host_list []HostInfo
	host_list = make([]HostInfo, 0)
	//блокируем  и снимаем при выходе из функции
	infra.HS.RLock()
	defer infra.HS.RUnlock()

	for _, hi := range infra.HSList {
		hi_n := *hi
		hi_n.Perf = make([]HSPerf, len(hi.Perf))
		for i, perf_i := range hi.Perf {
			hi_n.Perf[i] = perf_i
		}
		host_list = append(host_list, hi_n)
	}

	sort.Sort(HSListID(host_list))
	return host_list, nil
}

//GetHostListFilter полцчить срез с данными хостов по фильтру
//
func (infra *VMInfrastructure) GetHostListFilter(vcenter, datacenter, cluster, hostname string) ([]HostInfo, error) {

	if infra == nil {
		return nil, errors.New(vminfraNotInitialised)
	}

	var host_list []HostInfo
	host_list = make([]HostInfo, 0)
	//блокируем  и снимаем при выходе из функции
	infra.HS.RLock()
	defer infra.HS.RUnlock()

	for _, hi := range infra.HSList {
		if vcenter != "all" && hi.Vcenter != vcenter {
			continue
		}
		if datacenter != "all" && hi.Datacenter != datacenter {
			continue
		}
		if cluster != "all" && hi.Cluster != cluster {
			continue
		}
		if hostname != "all" && hi.Name != hostname {
			continue
		}
		hi_n := *hi
		hi_n.Perf = make([]HSPerf, len(hi.Perf))
		for i, perf_i := range hi.Perf {
			hi_n.Perf[i] = perf_i
		}
		host_list = append(host_list, hi_n)
	}

	sort.Sort(HSListID(host_list))

	return host_list, nil
}

func (cl *Cluster) addHostPerfomance(hi HostInfo) {
	cl.CPUMhzAvg = cl.CPUMhzAvg + hi.CPUMhz
	cl.AllNumCPUPkgs = cl.AllNumCPUPkgs + hi.NumCPUPkgs
	cl.AllNumCPUCores = cl.AllNumCPUCores + hi.NumCPUCores
	cl.AllNumCPUThreads = cl.AllNumCPUThreads + hi.NumCPUThreads
	cl.AllMemorySizeGB = cl.AllMemorySizeGB + hi.MemorySizeGB
	cl.CPUUsageMaximum = cl.CPUUsageMaximum + hi.CPUUsageMaximum
	cl.CPUUsageAverage = cl.CPUUsageAverage + hi.CPUUsageAverage
	cl.CPUUsageMinimum = cl.CPUUsageMinimum + hi.CPUUsageMinimum
	cl.MEMUsageMaximum = cl.MEMUsageMaximum + hi.MEMUsageMaximum
	cl.MEMUsageAverage = cl.MEMUsageAverage + hi.MEMUsageAverage
	cl.MEMUsageMinimum = cl.MEMUsageMinimum + hi.MEMUsageMinimum
	cl.DiskUsageMaximum = cl.DiskUsageMaximum + hi.DiskUsageMaximum
	cl.DiskUsageAverage = cl.DiskUsageAverage + hi.DiskUsageAverage
	cl.DiskUsageMinimum = cl.DiskUsageMinimum + hi.DiskUsageMinimum
	cl.DiskTotalReadLatencyMaximum = cl.DiskTotalReadLatencyMaximum + hi.DiskTotalReadLatencyMaximum
	cl.DiskTotalReadLatencyAverage = cl.DiskTotalReadLatencyAverage + hi.DiskTotalReadLatencyAverage
	cl.DiskTotalReadLatencyMinimum = cl.DiskTotalReadLatencyMinimum + hi.DiskTotalReadLatencyMinimum
	cl.DiskTotalWriteLatencyMaximum = cl.DiskTotalWriteLatencyMaximum + hi.DiskTotalWriteLatencyMaximum
	cl.DiskTotalWriteLatencyAverage = cl.DiskTotalWriteLatencyAverage + hi.DiskTotalWriteLatencyAverage
	cl.DiskTotalWriteLatencyMinimum = cl.DiskTotalWriteLatencyMinimum + hi.DiskTotalWriteLatencyMinimum
	cl.DiskWriteMaximum = cl.DiskWriteMaximum + hi.DiskWriteMaximum
	cl.DiskWriteAverage = cl.DiskWriteAverage + hi.DiskWriteAverage
	cl.DiskWriteMinimum = cl.DiskWriteMinimum + hi.DiskWriteMinimum
	cl.DiskReadMaximum = cl.DiskReadMaximum + hi.DiskReadMaximum
	cl.DiskReadAverage = cl.DiskReadAverage + hi.DiskReadAverage
	cl.DiskReadMinimum = cl.DiskReadMinimum + hi.DiskReadMinimum
	cl.NetUsageMaximum = cl.NetUsageMaximum + hi.NetUsageMaximum
	cl.NetUsageAverage = cl.NetUsageAverage + hi.NetUsageAverage
	cl.NetUsageMinimum = cl.NetUsageMinimum + hi.NetUsageMinimum
	cl.CPUBilling = cl.CPUBilling + hi.CPUBilling
	cl.DiskBilling = cl.DiskBilling + hi.DiskBilling
	cl.MEMBilling = cl.MEMBilling + hi.MEMBilling
	cl.NetBilling = cl.NetBilling + hi.NetBilling
}

func (cl *Cluster) calcHostPerfomance() {
	count := float64(len(cl.Hosts))
	if len(cl.Hosts) == 0 {
		return
	}

	cl.CPUMhzAvg = int32(float64(cl.CPUMhzAvg) / count)
	cl.CPUUsageMaximum = cl.CPUUsageMaximum / count
	cl.CPUUsageAverage = cl.CPUUsageAverage / count
	cl.CPUUsageMinimum = cl.CPUUsageMinimum / count
	cl.MEMUsageMaximum = cl.MEMUsageMaximum / count
	cl.MEMUsageAverage = cl.MEMUsageAverage / count
	cl.MEMUsageMinimum = cl.MEMUsageMinimum / count
	cl.DiskUsageMaximum = cl.DiskUsageMaximum
	cl.DiskUsageAverage = cl.DiskUsageAverage
	cl.DiskUsageMinimum = cl.DiskUsageMinimum
	cl.DiskTotalReadLatencyMaximum = cl.DiskTotalReadLatencyMaximum
	cl.DiskTotalReadLatencyAverage = cl.DiskTotalReadLatencyAverage
	cl.DiskTotalReadLatencyMinimum = cl.DiskTotalReadLatencyMinimum
	cl.DiskTotalWriteLatencyMaximum = cl.DiskTotalWriteLatencyMaximum
	cl.DiskTotalWriteLatencyAverage = cl.DiskTotalWriteLatencyAverage
	cl.DiskTotalWriteLatencyMinimum = cl.DiskTotalWriteLatencyMinimum
	cl.DiskWriteMaximum = cl.DiskWriteMaximum
	cl.DiskWriteAverage = cl.DiskWriteAverage
	cl.DiskWriteMinimum = cl.DiskWriteMinimum
	cl.DiskReadMaximum = cl.DiskReadMaximum
	cl.DiskReadAverage = cl.DiskReadAverage
	cl.DiskReadMinimum = cl.DiskReadMinimum
	cl.NetUsageMaximum = cl.NetUsageMaximum
	cl.NetUsageAverage = cl.NetUsageAverage
	cl.NetUsageMinimum = cl.NetUsageMinimum
}

func (cl *Cluster) calcPerfomance() {
	if len(cl.Hosts) == 0 {
		return
	}
	maxCount := int64(0)
	cl.CPUUsageMinimum = initFloat64min
	cl.MEMUsageMinimum = initFloat64min
	cl.DiskUsageMinimum = initFloat64min
	cl.DiskTotalReadLatencyMinimum = initFloat64min
	cl.DiskTotalWriteLatencyMinimum = initFloat64min
	cl.DiskWriteMinimum = initFloat64min
	cl.DiskReadMinimum = initFloat64min
	cl.NetUsageMinimum = initFloat64min

	for _, hs := range cl.Hosts {
		if hs.PerfCount > maxCount {
			maxCount = hs.PerfCount
		}

		cl.CPUMhzAvg += (hs.CPUMhz * int32(hs.NumCPUCores))
		cl.AllNumCPUPkgs += hs.NumCPUPkgs
		cl.AllNumCPUCores += hs.NumCPUCores
		cl.AllNumCPUThreads += hs.NumCPUThreads
		cl.AllMemorySizeGB += hs.MemorySizeGB

		if len(hs.Perf) == 0 {
			continue
		}

		cl.CPUBilling += hs.CPUBilling
		cl.DiskBilling += hs.DiskBilling
		cl.MEMBilling += hs.MEMBilling
		cl.NetBilling += hs.NetBilling
	}

	if cl.AllNumCPUCores > 0 {
		cl.CPUMhzAvg = cl.CPUMhzAvg / int32(cl.AllNumCPUCores)
	}

	cl.Perf = make([]CLPerf, maxCount)

	for i := 0; i < int(maxCount); i++ {
		cl.Perf[i].ID = int64(i)
		cl.Perf[i].CPUUsage = 0.0
		cl.Perf[i].MEMUsage = 0.0
		cl.Perf[i].DiskUsage = 0.0
		cl.Perf[i].DiskTotalReadLatency = 0.0
		cl.Perf[i].DiskTotalWriteLatency = 0.0
		cl.Perf[i].DiskWrite = 0.0
		cl.Perf[i].DiskRead = 0.0
		cl.Perf[i].NetUsage = 0.0

		for _, hs := range cl.Hosts {
			if i < len(hs.Perf) {
				if cl.Perf[i].Start.Equal(time.Time{}) {
					cl.Perf[i].Start = hs.Perf[i].Start // проверить
				}
				cl.Perf[i].CPUUsage += hs.Perf[i].CPUUsage
				cl.Perf[i].MEMUsage += hs.Perf[i].MEMUsage
				cl.Perf[i].DiskUsage += hs.Perf[i].DiskUsage
				cl.Perf[i].DiskTotalReadLatency += hs.Perf[i].DiskTotalReadLatency
				cl.Perf[i].DiskTotalWriteLatency += hs.Perf[i].DiskTotalWriteLatency
				cl.Perf[i].DiskWrite += hs.Perf[i].DiskWrite
				cl.Perf[i].DiskRead += hs.Perf[i].DiskRead
				cl.Perf[i].NetUsage += hs.Perf[i].NetUsage
			}
		}
		if len(cl.Hosts) > 0 {
			cl.Perf[i].CPUUsage /= float64(len(cl.Hosts))
			cl.Perf[i].MEMUsage /= float64(len(cl.Hosts))
		}
		if cl.CPUUsageMaximum < cl.Perf[i].CPUUsage {
			cl.CPUUsageMaximum = cl.Perf[i].CPUUsage
			cl.CPUUsageMaximumTime = cl.Perf[i].Start
		}
		if cl.CPUUsageMinimum > cl.Perf[i].CPUUsage {
			cl.CPUUsageMinimum = cl.Perf[i].CPUUsage
			cl.CPUUsageMinimumTime = cl.Perf[i].Start
		}
		cl.CPUUsageAverage += cl.Perf[i].CPUUsage

		if cl.MEMUsageMaximum < cl.Perf[i].MEMUsage {
			cl.MEMUsageMaximum = cl.Perf[i].MEMUsage
			cl.MEMUsageMaximumTime = cl.Perf[i].Start
		}
		if cl.MEMUsageMinimum > cl.Perf[i].MEMUsage {
			cl.MEMUsageMinimum = cl.Perf[i].MEMUsage
			cl.MEMUsageMinimumTime = cl.Perf[i].Start
		}
		cl.MEMUsageAverage += cl.Perf[i].MEMUsage

		if cl.DiskUsageMaximum < cl.Perf[i].DiskUsage {
			cl.DiskUsageMaximum = cl.Perf[i].DiskUsage
			cl.DiskUsageMaximumTime = cl.Perf[i].Start
		}
		if cl.DiskUsageMinimum > cl.Perf[i].DiskUsage {
			cl.DiskUsageMinimum = cl.Perf[i].DiskUsage
			cl.DiskUsageMinimumTime = cl.Perf[i].Start
		}
		cl.DiskUsageAverage += cl.Perf[i].DiskUsage

		if cl.DiskWriteMaximum < cl.Perf[i].DiskWrite {
			cl.DiskWriteMaximum = cl.Perf[i].DiskWrite
			cl.DiskWriteMaximumTime = cl.Perf[i].Start
		}
		if cl.DiskWriteMinimum > cl.Perf[i].DiskWrite {
			cl.DiskWriteMinimum = cl.Perf[i].DiskWrite
			cl.DiskWriteMinimumTime = cl.Perf[i].Start
		}
		cl.DiskWriteAverage += cl.Perf[i].DiskWrite

		if cl.DiskReadMaximum < cl.Perf[i].DiskRead {
			cl.DiskReadMaximum = cl.Perf[i].DiskRead
			cl.DiskReadMaximumTime = cl.Perf[i].Start
		}

		if cl.DiskReadMinimum > cl.Perf[i].DiskRead {
			cl.DiskReadMinimum = cl.Perf[i].DiskRead
			cl.DiskReadMinimumTime = cl.Perf[i].Start
		}

		cl.DiskReadAverage += cl.Perf[i].DiskRead

		if cl.DiskTotalReadLatencyMaximum < cl.Perf[i].DiskTotalReadLatency {
			cl.DiskTotalReadLatencyMaximum = cl.Perf[i].DiskTotalReadLatency
			cl.DiskTotalReadLatencyMaximumTime = cl.Perf[i].Start
		}
		if cl.DiskTotalReadLatencyMinimum > cl.Perf[i].DiskTotalReadLatency {
			cl.DiskTotalReadLatencyMinimum = cl.Perf[i].DiskTotalReadLatency
			cl.DiskTotalReadLatencyMinimumTime = cl.Perf[i].Start
		}
		cl.DiskTotalReadLatencyAverage += cl.Perf[i].DiskTotalReadLatency

		if cl.DiskTotalWriteLatencyMaximum < cl.Perf[i].DiskTotalWriteLatency {
			cl.DiskTotalWriteLatencyMaximum = cl.Perf[i].DiskTotalWriteLatency
			cl.DiskTotalWriteLatencyMaximumTime = cl.Perf[i].Start
		}
		if cl.DiskTotalWriteLatencyMinimum > cl.Perf[i].DiskTotalWriteLatency {
			cl.DiskTotalWriteLatencyMinimum = cl.Perf[i].DiskTotalWriteLatency
			cl.DiskTotalWriteLatencyMinimumTime = cl.Perf[i].Start
		}
		cl.DiskTotalWriteLatencyAverage += cl.Perf[i].DiskTotalWriteLatency

		if cl.NetUsageMaximum < cl.Perf[i].NetUsage {
			cl.NetUsageMaximum = cl.Perf[i].NetUsage
			cl.NetUsageMaximumTime = cl.Perf[i].Start
		}
		if cl.NetUsageMinimum > cl.Perf[i].NetUsage {
			cl.NetUsageMinimum = cl.Perf[i].NetUsage
			cl.NetUsageMinimumTime = cl.Perf[i].Start
		}
		cl.NetUsageAverage += cl.Perf[i].NetUsage
	}
	switch {
	case maxCount > 0:
		cl.CPUUsageAverage /= float64(maxCount)
		cl.MEMUsageAverage /= float64(maxCount)
		cl.DiskUsageAverage /= float64(maxCount)
		cl.DiskWriteAverage /= float64(maxCount)
		cl.DiskReadAverage /= float64(maxCount)
		cl.DiskTotalReadLatencyAverage /= float64(maxCount)
		cl.DiskTotalWriteLatencyAverage /= float64(maxCount)
		cl.NetUsageAverage /= float64(maxCount)
	default:
		cl.CPUUsageMaximum = 0.0
		cl.CPUUsageAverage = 0.0
		cl.CPUUsageMinimum = 0.0
		cl.MEMUsageMaximum = 0.0
		cl.MEMUsageAverage = 0.0
		cl.MEMUsageMinimum = 0.0
		cl.DiskUsageMaximum = 0.0
		cl.DiskUsageAverage = 0.0
		cl.DiskUsageMinimum = 0.0
		cl.DiskTotalReadLatencyMaximum = 0.0
		cl.DiskTotalReadLatencyAverage = 0.0
		cl.DiskTotalReadLatencyMinimum = 0.0
		cl.DiskTotalWriteLatencyMaximum = 0.0
		cl.DiskTotalWriteLatencyAverage = 0.0
		cl.DiskTotalWriteLatencyMinimum = 0.0
		cl.DiskWriteMaximum = 0.0
		cl.DiskWriteAverage = 0.0
		cl.DiskWriteMinimum = 0.0
		cl.DiskReadMaximum = 0.0
		cl.DiskReadAverage = 0.0
		cl.DiskReadMinimum = 0.0
		cl.NetUsageMaximum = 0.0
		cl.NetUsageAverage = 0.0
		cl.NetUsageMinimum = 0.0
	}
}

//GetClusterListFilter получить срез указателей на кластеры по фильтру
//
func (infra *VMInfrastructure) GetClusterListFilter(vcenter, datacenter, cluster string) ([]*Cluster, error) {
	if infra == nil {
		return nil, errors.New(vminfraNotInitialised)
	}
	clusterList := make([]*Cluster, 0)
	//блокируем  и снимаем при выходе из функции
	infra.CL.RLock()
	defer infra.CL.RUnlock()

	for _, cl := range infra.CLList {
		if vcenter != "all" && cl.Vcenter != vcenter {
			continue
		}
		if datacenter != "all" && cl.Datacenter != datacenter {
			continue
		}
		if cluster != "all" && cl.Name != cluster {
			continue
		}
		cl_n := *cl
		cl_n.Hosts = make([]*HostInfo, len(cl.Hosts))
		cl_n.HostCount = int64(len(cl.Hosts))

		cl_n.VM = make([]*VMInfo, 0)

		for _, vm := range cl.VM {
			vmNew := *vm
			vmNew.Perf = make([]VMPerf, len(vm.Perf))
			for i, perf := range vm.Perf {
				vmNew.Perf[i] = perf
			}
			vmNew.VMDisk = make([]GuestDiskInfo, len(vm.VMDisk))
			for i, disk := range vm.VMDisk {
				vmNew.VMDisk[i] = disk
			}

			vmNew.UsageDatastore = make([]VMUsageOnDatastore, len(vm.UsageDatastore))
			for i, st := range vm.UsageDatastore {
				vmNew.UsageDatastore[i] = st
			}

			count := float64(vm.PerfCount)
			if vm.PerfCount > 0 {
				vmNew.CPUUsageAverage = vm.CPUUsageAverage / count
				vmNew.MEMUsageAverage = vm.MEMUsageAverage / count
				vmNew.DiskReadAverage = vm.DiskReadAverage / count
				vmNew.DiskWriteAverage = vm.DiskWriteAverage / count
				vmNew.DiskUsageAverage = vm.DiskUsageAverage / count
				vmNew.NetUsageAverage = vm.NetUsageAverage / count
			} else {
				vmNew.CPUUsageAverage = 0.0
				vmNew.MEMUsageAverage = 0.0
				vmNew.DiskReadAverage = 0.0
				vmNew.DiskWriteAverage = 0.0
				vmNew.DiskUsageAverage = 0.0
				vmNew.NetUsageAverage = 0.0
			}

			switch {
			case vm.PerfCount > 0:
				vmNew.CPUUsageAverage = vm.CPUUsageAverage / count
				vmNew.MEMUsageAverage = vm.MEMUsageAverage / count
				vmNew.DiskReadAverage = vm.DiskReadAverage / count
				vmNew.DiskWriteAverage = vm.DiskWriteAverage / count
				vmNew.DiskUsageAverage = vm.DiskUsageAverage / count
				vmNew.NetUsageAverage = vm.NetUsageAverage / count
			default:
				vmNew.CPUUsageMaximum = 0.0
				vmNew.CPUUsageAverage = 0.0
				vmNew.CPUUsageMinimum = 0.0
				vmNew.MEMUsageMaximum = 0.0
				vmNew.MEMUsageAverage = 0.0
				vmNew.MEMUsageMinimum = 0.0
				vmNew.DiskUsageMaximum = 0.0
				vmNew.DiskUsageAverage = 0.0
				vmNew.DiskUsageMinimum = 0.0
				vmNew.DiskTotalReadLatencyMaximum = 0.0
				vmNew.DiskTotalReadLatencyAverage = 0.0
				vmNew.DiskTotalReadLatencyMinimum = 0.0
				vmNew.DiskTotalWriteLatencyMaximum = 0.0
				vmNew.DiskTotalWriteLatencyAverage = 0.0
				vmNew.DiskTotalWriteLatencyMinimum = 0.0
				vmNew.DiskWriteMaximum = 0.0
				vmNew.DiskWriteAverage = 0.0
				vmNew.DiskWriteMinimum = 0.0
				vmNew.DiskReadMaximum = 0.0
				vmNew.DiskReadAverage = 0.0
				vmNew.DiskReadMinimum = 0.0
				vmNew.NetUsageMaximum = 0.0
				vmNew.NetUsageAverage = 0.0
				vmNew.NetUsageMinimum = 0.0
			}

			cl_n.VM = append(cl_n.VM, &vmNew)
		}

		for i, hi := range cl.Hosts {
			count := float64(hi.PerfCount)
			hi_n := *hi
			switch {
			case hi.PerfCount > 0:
				hi_n.CPUUsageAverage = hi.CPUUsageAverage / count
				hi_n.MEMUsageAverage = hi.MEMUsageAverage / count
				hi_n.DiskReadAverage = hi.DiskReadAverage / count
				hi_n.DiskWriteAverage = hi.DiskWriteAverage / count
				hi_n.DiskUsageAverage = hi.DiskUsageAverage / count
				hi_n.NetUsageAverage = hi.NetUsageAverage / count
			default:
				hi_n.CPUUsageMaximum = 0.0
				hi_n.CPUUsageAverage = 0.0
				hi_n.CPUUsageMinimum = 0.0

				hi_n.MEMUsageMaximum = 0.0
				hi_n.MEMUsageAverage = 0.0
				hi_n.MEMUsageMinimum = 0.0

				hi_n.DiskReadMaximum = 0.0
				hi_n.DiskReadAverage = 0.0
				hi_n.DiskReadMinimum = 0.0

				hi_n.DiskWriteMaximum = 0.0
				hi_n.DiskWriteAverage = 0.0
				hi_n.DiskWriteMinimum = 0.0

				hi_n.DiskUsageMaximum = 0.0
				hi_n.DiskUsageAverage = 0.0
				hi_n.DiskUsageMinimum = 0.0

				hi_n.NetUsageMaximum = 0.0
				hi_n.NetUsageAverage = 0.0
				hi_n.NetUsageMinimum = 0.0
			}

			hi_n.Perf = make([]HSPerf, 0)
			for _, perf := range hi.Perf {
				perfNew := perf
				hi_n.Perf = append(hi_n.Perf, perfNew)
			}
			hi_n.PerfCount = int64(len(hi_n.Perf))

			cl_n.Hosts[i] = &hi_n
		}
		cl_n.calcPerfomance()
		sort.Sort(HSListName(cl.Hosts))
		cl_n.Storage = make([]*Datastore, 0)
		for _, st := range cl.Storage {
			stNew := *st
			cl_n.Storage = append(cl_n.Storage, &stNew)
		}
		clusterList = append(clusterList, &cl_n)
	}

	sort.Sort(ClusterCPUPerf(clusterList))

	return clusterList, nil
}

//GetVMList получить срез всех виртуальных машин со всех хатегистрированных vcenter
//
func (infra *VMInfrastructure) GetVMList() ([]VMInfo, error) {
	if infra == nil {
		return nil, errors.New(vminfraNotInitialised)
	}

	var vm_list []VMInfo
	vm_list = make([]VMInfo, 0)
	//блокируем  и снимаем при выходе из функции
	infra.VM.RLock()
	defer infra.VM.RUnlock()

	for _, vm := range infra.VMList {
		vm_n := *vm
		vm_n.Perf = make([]VMPerf, len(vm.Perf))
		for i, perf_i := range vm.Perf {
			vm_n.Perf[i] = perf_i
		}
		vm_list = append(vm_list, vm_n)
		runtime.Gosched()
	}
	sort.Sort(VMListID(vm_list))
	return vm_list, nil
}

//GetVMListFilter получить срез всех виртуальных машин со всех хатегистрированных vcenter, с отбором по фильтру
//
func (infra *VMInfrastructure) GetVMListFilter(vcenter, datacenter, cluster string, vmname string) ([]VMInfo, error) {
	if infra == nil {
		return nil, errors.New(vminfraNotInitialised)
	}

	var vm_list []VMInfo
	vm_list = make([]VMInfo, 0)
	//блокируем  и снимаем при выходе из функции
	infra.VM.RLock()
	defer infra.VM.RUnlock()

	for _, vm := range infra.VMList {

		if vcenter != "all" && vcenter != "" && vm.Vcenter != vcenter {
			continue
		}
		if datacenter != "all" && datacenter != "" && vm.Datacenter != datacenter {
			continue
		}
		if cluster != "all" && cluster != "" && vm.Cluster != cluster {
			continue
		}
		if vmname != "all" && vmname != "" && vm.VMName != vmname {
			continue
		}

		vm_n := *vm
		vm_n.Perf = make([]VMPerf, len(vm.Perf))
		for i, perf_i := range vm.Perf {
			vm_n.Perf[i] = perf_i
		}
		vm_list = append(vm_list, vm_n)
		runtime.Gosched()
	}
	sort.Sort(VMListID(vm_list))
	return vm_list, nil
}

//GetVMPerfomance получить все метрики для виртуальной машины с указанным ID
//
func (infra *VMInfrastructure) GetVMPerfomance(id int64) *VMInfo {

	if infra == nil {
		return nil
	}

	vmp := VMInfo{}
	vmp.Perf = make([]VMPerf, 0)

	infra.VM.RLock()
	defer infra.VM.RUnlock()

	for _, vm := range infra.VMList {
		if vm.ID == id {
			vmp = *vm

			vmp.Perf = make([]VMPerf, len(vm.Perf))
			for j, perf_i := range vm.Perf {
				vmp.Perf[j] = perf_i
				runtime.Gosched()
			}
			return &vmp
		}
	}
	return &vmp
}

//GetVMPerfomanceName получить все метрики для виртуальной машины с указанным именем
//
func (infra *VMInfrastructure) GetVMPerfomanceName(name string) *VMInfo {

	if infra == nil {
		return nil
	}

	vmp := VMInfo{}
	vmp.Perf = make([]VMPerf, 0)

	infra.VM.RLock()
	defer infra.VM.RUnlock()

	for _, vm := range infra.VMList {
		if strings.Contains(vm.VMHostName, name) {
			vmp = *vm

			vmp.Perf = make([]VMPerf, len(vm.Perf))
			for j, perf_i := range vm.Perf {
				vmp.Perf[j] = perf_i
				runtime.Gosched()
			}
			return &vmp
		}
	}
	return &vmp
}

//connect подключиться к vcenter
func (vcenter *VCenter) connect() (*govmomi.Client, error) {

	if vcenter == nil {
		return nil, errors.New("ошибка VCenter не инициализирован")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log.Printf("vcenter %s: Подключение\n", vcenter.Hostname)
	username := url.QueryEscape(vcenter.Username)
	password := url.QueryEscape(vcenter.Password)
	u, err := url.Parse("https://" + username + ":" + password + "@" + vcenter.Hostname + "/sdk")
	if err != nil {
		log.Printf("vcenter %s: ошибка разбора строки кодключения vcenter - %s\n", vcenter.Hostname, err)
		return nil, err
	}
	client, err := govmomi.NewClient(ctx, u, true) //insecure false
	if err != nil {
		log.Printf("vcenter %s: невозможно подключиться к vcenter - %s\n", vcenter.Hostname, err)
		return nil, err
	}
	return client, nil
}

//Init инициализация инфраструктуры
func (infra *VMInfrastructure) Init() {
	if infra == nil {
		fmt.Println("func (infra	*VMInfrastructure)Init()", errors.New(vminfraNotInitialised))
		return
	}
	infra.VCList = make([]VCenter, 0)
	infra.VMList = make(map[string]*VMInfo)
	infra.HSList = make(map[string]*HostInfo)
	infra.DSList = make(map[string]*Datastore)
}

//Destroy уничтожение объектов инфраструктуры
func (infra *VMInfrastructure) Destroy() {

	if infra == nil {
		return
	}
	infrastructureGlobal.IN.Lock()
	infrastructureGlobal.Initialized = false
	infrastructureGlobal.IN.Unlock()

	infra.VC.Lock()
	infra.CL.Lock()
	infra.VM.Lock()
	infra.HS.Lock()

	for key, _ := range infra.VMList {
		//               vm.Perf = nil
		delete(infra.VMList, key)
	}

	for key, _ := range infra.HSList {
		//               hs.Perf = nil
		delete(infra.HSList, key)
	}

	infra.VCList = make([]VCenter, 0)
	for i, _ := range infra.CLList {
		infra.CLList[i].Hosts = make([]*HostInfo, 0)
	}
	infra.CLList = make([]*Cluster, 0)
	infra.VMList = make(map[string]*VMInfo)
	infra.HSList = make(map[string]*HostInfo)

	runtime.GC()
	infra.HS.Unlock()
	infra.VM.Unlock()
	infra.CL.Unlock()
	infra.VC.Unlock()
}

//DestroyPerfomance очистка структур метрик
func (infra *VMInfrastructure) DestroyPerfomance() {

	if infra == nil {
		return
	}
	infra.VC.Lock()
	infra.VM.Lock()
	infra.HS.Lock()
	infra.CL.Lock()

	for _, vm := range infra.VMList {
		vm.vmClearPerfomance()
		vm.Perf = make([]VMPerf, 0)
	}

	for _, hs := range infra.HSList {
		hs.hsClearPerfomance()
		hs.Perf = make([]HSPerf, 0)
	}
	currentTime := time.Now().Local()
	for _, cl := range infra.CLList {
		cl.clClearPerfomance()
		cl.Date = currentTime //инициализируем Дату и время в структуре кластера
	}
	runtime.GC()
	infra.CL.Unlock()
	infra.HS.Unlock()
	infra.VM.Unlock()
	infra.VC.Unlock()
}

//AddVCenter добавить управляющий центр в список опрашиваемых
func (infra *VMInfrastructure) AddVCenter(hostname string, username string, password string) error {

	if infra == nil {
		return errors.New(vminfraNotInitialised)
	}

	infra.VC.Lock()
	infra.VCList = append(infra.VCList, VCenter{Hostname: hostname, Username: username, Password: password})
	infra.VC.Unlock()
	return nil
}

//LoadVMInfo загрузить информацию по всем виртуальным машинам из управлющих центров в инфраструктуру
func (infra *VMInfrastructure) LoadVMInfo() error {

	if infra == nil {
		return errors.New(vminfraNotInitialised)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Id_VM := int64(0)

	for _, vc := range infra.VCList {
		c, err := vc.connect()
		if err != nil {
			log.Printf("Ошибка подключения: %s\n", err.Error())
			continue
		}
		defer c.Logout(ctx)

		f := find.NewFinder(c.Client, true)

		dclist, err := f.DatacenterList(ctx, "*")
		if err != nil {
			fmt.Println("Список датацентров", err)
			return err
		}

		for _, dc := range dclist {

			dcname, err := dc.ObjectName(ctx)
			if err != nil {
				fmt.Println("Запрос имени датацентра", err)
				return err
			}

			f.SetDatacenter(dc)

			vms, err := f.VirtualMachineList(ctx, "*")
			if err != nil {
				fmt.Println("Поиск виртуальных машин", err)
				return err
			}

			pc := property.DefaultCollector(c.Client)

			var refs []types.ManagedObjectReference

			for _, vm := range vms {
				//fmt.Printf("ref %v\n", vm.Reference())
				refs = append(refs, vm.Reference())
				runtime.Gosched()
			}

			var vmt []mo.VirtualMachine
			//	              err = pc.Retrieve(ctx, refs, []string{"name", "summary","hardware","network", "datastore", "storage", "guest.ipAddress", "config.extraConfig", "config.tools"}, &vmt)       //добавить  , "tags"
			err = pc.Retrieve(ctx, refs, nil, &vmt)
			if err != nil {
				fmt.Println("Поиск свойств всех виртуальных машин", err)
				return err
			}

			sort.Sort(ByName(vmt))
			//блокируем
			infra.VM.Lock()
			for _, vm := range vmt {
				runtime.Gosched()
				if strings.Contains(vm.Summary.Config.GuestId, "windows") {
					if !strings.Contains(vm.Summary.Config.GuestId, "Server") {
						continue
					}
				}
				//получение дополнительной информации (снапшоты, логи ...)
				if vm.Snapshot != nil { //есть snapshot
					_ = vm.Snapshot.CurrentSnapshot  //types.VirtualMachineSnapshotInfo
					_ = vm.Snapshot.RootSnapshotList // types.VirtualMachineSnapshotTree

				}
				notCluster := "VM не в кластере"
				clname := notCluster

				if href := vm.Summary.Runtime.Host; href != nil {
					if hi, ok := infra.HSList[vc.Hostname+"/"+href.Value]; ok { //
						clname = hi.Cluster
						if clname == "" {
							clname = hi.Name //если это одиночный хост
						}
					}
				}
				var cl *Cluster = nil
				if clname != notCluster {
					cl, err = infra.getCluster(clname)
				}

				vmp := VMInfo{ID: Id_VM, Vcenter: vc.Hostname, Datacenter: dcname, Cluster: clname}
				Id_VM++

				vmp.CPUUsageMinimum = initFloat64min
				vmp.MEMUsageMinimum = initFloat64min
				vmp.DiskUsageMinimum = initFloat64min
				vmp.DiskTotalReadLatencyMinimum = initFloat64min
				vmp.DiskTotalWriteLatencyMinimum = initFloat64min
				vmp.DiskWriteMinimum = initFloat64min
				vmp.DiskReadMinimum = initFloat64min
				vmp.NetUsageMinimum = initFloat64min

				vmp.importData(vm, infra)
				vmp.KeyName = vmp.Vcenter + "/" + vm.Reference().Value
				if cl != nil {
					cl.VM = append(cl.VM, &vmp)
					cl.VMCount++
				}
				infra.VMList[vmp.Vcenter+"/"+vm.Reference().Value] = &vmp
			}
			//снимаем блокировку
			infra.VM.Unlock()
		}
	}
	infra.CL.Lock()
	infra.VM.Lock()
	for _, cl := range infra.CLList {
		for _, st := range cl.Storage {
			for _, vmDescr := range st.vmnames {
				vmp := infra.VMList[cl.Vcenter+"/"+vmDescr.Value]
				if vmp != nil {
					st.VM = append(st.VM, vmp)
					st.VMCount++
				}
			}
		}
	}
	infra.VM.Unlock()
	infra.CL.Unlock()
	infrastructureGlobal.IN.Lock()
	infrastructureGlobal.Initialized = true
	infrastructureGlobal.IN.Unlock()
	return nil
}

//LoadHostAndVMMetric - загрузка метрик по хостам и виртуальным машинам + симуляция метрик по кластерам
func (infra *VMInfrastructure) LoadHostAndVMMetric() error {

	if infra == nil {
		return errors.New(vminfraNotInitialised)
	}
	infrastructureGlobal.IN.RLock()
	defer infrastructureGlobal.IN.RUnlock()
	if !infrastructureGlobal.Initialized {
		return errors.New("Can not initialized infrastructure. Load CL,HS and VM info.")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer setMetric() //обновляем значения метрик

	for _, vc := range infra.VCList {
		c, err := vc.connect()
		if err != nil {
			log.Printf("Ошибка подключения: ", err)
			continue
		}
		defer c.Logout(ctx)

		m2 := view.NewManager(c.Client)

		v2, err := m2.CreateContainerView(ctx, c.Client.ServiceContent.RootFolder, nil, true)
		if err != nil {
			fmt.Println("Ошибка CreateContainerView:", err)
			return err
		}

		defer v2.Destroy(ctx)

		hostRefs, errh := v2.Find(ctx, []string{"HostSystem"}, nil)
		if errh != nil {
			fmt.Println("Ошибка Find HostSystem:", errh)
			return errh
		}

		vmsRefs, err := v2.Find(ctx, []string{"VirtualMachine"}, nil)
		if err != nil {
			fmt.Println("Ошибка Find VirtualMachine:", err)
			return err
		}

		// Create a PerfManager
		perfManager := performance.NewManager(c.Client)

		// Create PerfQuerySpec
		spec := types.PerfQuerySpec{
			MaxSample:  2,
			MetricId:   []types.PerfMetricId{{Instance: "*"}},
			IntervalId: 20,
		}

		// Запрос необходимого набора метрик
		sampleHosts, errh := perfManager.SampleByName(ctx, spec, []string{"cpu.usage.average", "mem.usage.average",
			"disk.deviceReadLatency.average", "disk.deviceWriteLatency.average",
			"disk.write.average", "disk.read.average", "net.usage.average",
			"disk.usage.average"}, hostRefs)
		if errh != nil {
			fmt.Println("perfManager.SampleByName HostSystem:", errh)
			return errh
		}

		resultHosts, errh := perfManager.ToMetricSeries(ctx, sampleHosts)
		if errh != nil {
			fmt.Println("perfManager.ToMetricSeries HostSystem:", errh)
			return errh
		}
		fmt.Printf("количество метрик по хостам: %d\n", len(resultHosts))

		sample, err := perfManager.SampleByName(ctx, spec, []string{"cpu.usage.average", "mem.usage.average",
			"virtualDisk.totalReadLatency.average", "virtualDisk.totalWriteLatency.average",
			"virtualDisk.write.average", "virtualDisk.read.average", "net.usage.average",
			"disk.usage.average"}, vmsRefs)
		if err != nil {
			fmt.Println("perfManager.SampleByName VirtualMachine:", err)
			return err
		}

		result, err := perfManager.ToMetricSeries(ctx, sample)
		if err != nil {
			fmt.Println("perfManager.ToMetricSeries VirtualMachine:", err)
			return err
		}

		infra.HS.Lock()
		for _, metric := range resultHosts {
			if hi, ok := infra.HSList[vc.Hostname+"/"+metric.Entity.Value]; ok { //вариант с добавлением в ключ имени vcenter
				perf := HSPerf{CPUUsage: 0.0,
					MEMUsage:              0.0,
					DiskUsage:             0.0,
					DiskTotalReadLatency:  0.0,
					DiskTotalWriteLatency: 0.0,
					DiskWrite:             0.0,
					DiskRead:              0.0,
					NetUsage:              0.0}

				if len(metric.SampleInfo) > 0 {
					perf.Start = metric.SampleInfo[0].Timestamp.Local()
				} // время среза метрик и интервал по первой записи
				if len(hi.Perf) == 0 {
					hi.Perf = make([]HSPerf, 0)
				}

				for _, v := range metric.Value {
					if len(v.Value) > 0 {
						//fmt.Printf("1 - %s\n", v.Name)
						switch v.Name {
						case "cpu.usage.average":
							if v.Instance == "" {
								perf.CPUUsage = float64(v.Value[0]) / 100.0
							}
						case "mem.usage.average":
							perf.MEMUsage = float64(v.Value[0]) / 100.0
						case "disk.usage.average":
							perf.DiskUsage = float64(v.Value[0]) / 1024.0
						case "net.usage.average":
							perf.NetUsage = float64(v.Value[0]) / 1024.0
						case "disk.deviceReadLatency.average":
							perf.DiskTotalReadLatency = float64(v.Value[0]) //ms
						case "disk.deviceWriteLatency.average":
							perf.DiskTotalWriteLatency = float64(v.Value[0]) //ms
						case "disk.write.average":
							perf.DiskWrite = float64(v.Value[0]) / 1024.0
						case "disk.read.average":
							perf.DiskRead = float64(v.Value[0]) / 1024.0
						}
					}
				}
				if math.IsNaN(perf.CPUUsage) {
					perf.CPUUsage = 0.0
				}
				if math.IsNaN(perf.MEMUsage) {
					perf.MEMUsage = 0.0
				}
				if math.IsNaN(perf.DiskUsage) {
					perf.DiskUsage = 0.0
				}
				if math.IsNaN(perf.NetUsage) {
					perf.NetUsage = 0.0
				}
				if math.IsNaN(perf.DiskTotalReadLatency) {
					perf.DiskTotalReadLatency = 0.0
				}
				if math.IsNaN(perf.DiskTotalWriteLatency) {
					perf.DiskTotalWriteLatency = 0.0
				}
				if math.IsNaN(perf.DiskWrite) {
					perf.DiskWrite = 0.0
				}
				if math.IsNaN(perf.DiskRead) {
					perf.DiskRead = 0.0
				}

				perf.ID = hi.ID //Id виртуалки в метрики

				hi.CPUBilling = hi.CPUBilling + float64(perf.CPUUsage)*float64(hi.CPUMhz)*float64(hi.NumCPUCores)/180.0 //утилизация процессора частота в мегагерцах умноженная процент утилизации и промежуток времени 20 секунд (процент в виде целого умноженого на 100)
				hi.MEMBilling = hi.MEMBilling + float64(perf.MEMUsage)*hi.MemorySizeGB*1024.0/180.0                     // утилизация памяти мегабайт часах

				if hi.CPUUsageMaximum < perf.CPUUsage {
					hi.CPUUsageMaximum = perf.CPUUsage
					hi.CPUUsageMaximumTime = perf.Start
				}
				if hi.CPUUsageMinimum > perf.CPUUsage {
					hi.CPUUsageMinimum = perf.CPUUsage
					hi.CPUUsageMinimumTime = perf.Start
				}
				hi.CPUUsageAverage = hi.CPUUsageAverage + perf.CPUUsage

				if hi.MEMUsageMaximum < perf.MEMUsage {
					hi.MEMUsageMaximum = perf.MEMUsage
					hi.MEMUsageMaximumTime = perf.Start
				}
				if hi.MEMUsageMinimum > perf.MEMUsage {
					hi.MEMUsageMinimum = perf.MEMUsage
					hi.MEMUsageMinimumTime = perf.Start
				}
				hi.MEMUsageAverage = hi.MEMUsageAverage + perf.MEMUsage

				if hi.NetUsageMaximum < perf.NetUsage {
					hi.NetUsageMaximum = perf.NetUsage
					hi.NetUsageMaximumTime = perf.Start
				}
				if hi.NetUsageMinimum > perf.NetUsage {
					hi.NetUsageMinimum = perf.NetUsage
					hi.NetUsageMinimumTime = perf.Start
				}
				hi.NetUsageAverage = hi.NetUsageAverage + perf.NetUsage

				if hi.DiskUsageMaximum < perf.DiskUsage {
					hi.DiskUsageMaximum = perf.DiskUsage
					hi.DiskUsageMaximumTime = perf.Start
				}
				if hi.DiskUsageMinimum > perf.DiskUsage {
					hi.DiskUsageMinimum = perf.DiskUsage
					hi.DiskUsageMinimumTime = perf.Start
				}
				hi.DiskUsageAverage = hi.DiskUsageAverage + perf.DiskUsage

				if hi.DiskReadMaximum < perf.DiskRead {
					hi.DiskReadMaximum = perf.DiskRead
					hi.DiskReadMaximumTime = perf.Start
				}
				if hi.DiskReadMinimum > perf.DiskRead {
					hi.DiskReadMinimum = perf.DiskRead
					hi.DiskReadMinimumTime = perf.Start
				}
				hi.DiskReadAverage = hi.DiskReadAverage + perf.DiskRead

				if hi.DiskWriteMaximum < perf.DiskWrite {
					hi.DiskWriteMaximum = perf.DiskWrite
					hi.DiskWriteMaximumTime = perf.Start
				}
				if hi.DiskWriteMinimum > perf.DiskWrite {
					hi.DiskWriteMinimum = perf.DiskWrite
					hi.DiskWriteMinimumTime = perf.Start
				}
				hi.DiskWriteAverage = hi.DiskWriteAverage + perf.DiskWrite

				if hi.DiskTotalReadLatencyMaximum < perf.DiskTotalReadLatency {
					hi.DiskTotalReadLatencyMaximum = perf.DiskTotalReadLatency
					hi.DiskTotalReadLatencyMaximumTime = perf.Start
				}
				if hi.DiskTotalReadLatencyMinimum > perf.DiskTotalReadLatency {
					hi.DiskTotalReadLatencyMinimum = perf.DiskTotalReadLatency
					hi.DiskTotalReadLatencyMinimumTime = perf.Start
				}
				hi.DiskTotalReadLatencyAverage = hi.DiskTotalReadLatencyAverage + perf.DiskTotalReadLatency

				if hi.DiskTotalWriteLatencyMaximum < perf.DiskTotalWriteLatency {
					hi.DiskTotalWriteLatencyMaximum = perf.DiskTotalWriteLatency
					hi.DiskTotalWriteLatencyMaximumTime = perf.Start
				}
				if hi.DiskTotalWriteLatencyMinimum > perf.DiskTotalWriteLatency {
					hi.DiskTotalWriteLatencyMinimum = perf.DiskTotalWriteLatency
					hi.DiskTotalWriteLatencyMinimumTime = perf.Start
				}
				hi.DiskTotalWriteLatencyAverage = hi.DiskTotalWriteLatencyAverage + perf.DiskTotalWriteLatency

				hi.Perf = append(hi.Perf, perf) //добавить метрики
				hi.PerfCount++
			}
		}
		infra.HS.Unlock()

		fmt.Printf("количество метрик по виртуальным машинам: %d\n", len(result))

		infra.VM.Lock()

		for _, metric := range result {
			runtime.Gosched()
			//
			CpuMhz := 0.0
			pc := property.DefaultCollector(c.Client)

			var refs []types.ManagedObjectReference

			refs = append(refs, metric.Entity)

			var vmt []mo.VirtualMachine
			//	                        err = pc.Retrieve(ctx, refs, []string{"name", "config", "summary","hardware","network", "datastore", "storage","guest", "guest.ipAddress", "config.extraConfig", "config.tools"}, &vmt)       //добавить  , "tags"
			err = pc.Retrieve(ctx, refs, nil, &vmt)
			if err != nil {
				fmt.Println("Поиск свойств всех виртуальных машин", err)
				infra.VM.Unlock()
				return err
			}

			for _, vm := range vmt {
				runtime.Gosched()
				if vm.Summary.Runtime.PowerState == "poweredOff" {
					continue
				}
				if strings.Contains(vm.Summary.Config.GuestId, "windows") {
					if !strings.Contains(vm.Summary.Config.GuestId, "Server") {
						continue
					}
				}

				if href := vm.Summary.Runtime.Host; href != nil {
					if hi, ok := infra.HSList[vc.Hostname+"/"+href.Value]; ok { //рассмотреть вариант с добавлением в ключ имени vcenter
						CpuMhz = float64(hi.CPUMhz)
					}
				}
			}
			name := metric.Entity
			vm_p, ok := infra.VMList[vc.Hostname+"/"+metric.Entity.Value] //найти виртуальную машину
			if ok {
				//			   fmt.Println(name)
				perf := VMPerf{CPUUsage: 0.0,
					MEMUsage:              0.0,
					DiskUsage:             0.0,
					DiskTotalReadLatency:  0.0,
					DiskTotalWriteLatency: 0.0,
					DiskWrite:             0.0,
					DiskRead:              0.0,
					NetUsage:              0.0}
				if len(metric.SampleInfo) > 0 {
					perf.Start = metric.SampleInfo[0].Timestamp
				} // время среза метрик и интервал по первой записи
				if len(vm_p.Perf) == 0 {
					vm_p.Perf = make([]VMPerf, 0)
				}

				for _, v := range metric.Value {
					if len(v.Value) > 0 {
						//fmt.Printf("1 - %s\n", v.Name)
						switch v.Name {
						case "cpu.usage.average":
							if v.Instance == "" {
								perf.CPUUsage = float64(v.Value[0]) / 100.0
							}
						case "mem.usage.average":
							perf.MEMUsage = float64(v.Value[0]) / 100.0
						case "disk.usage.average":
							perf.DiskUsage = float64(v.Value[0]) / 1024.0
						case "net.usage.average":
							perf.NetUsage = float64(v.Value[0]) / 1024.0
						case "virtualDisk.totalReadLatency.average":
							perf.DiskTotalReadLatency = float64(v.Value[0]) //ms
						case "virtualDisk.totalWriteLatency.average":
							perf.DiskTotalWriteLatency = float64(v.Value[0]) //ms
						case "virtualDisk.write.average":
							perf.DiskWrite = float64(v.Value[0]) / 1024.0
						case "virtualDisk.read.average":
							perf.DiskRead = float64(v.Value[0]) / 1024.0
						}
					}
				}
				if math.IsNaN(perf.CPUUsage) {
					perf.CPUUsage = 0.0
				}
				if math.IsNaN(perf.MEMUsage) {
					perf.MEMUsage = 0.0
				}
				if math.IsNaN(perf.DiskUsage) {
					perf.DiskUsage = 0.0
				}
				if math.IsNaN(perf.NetUsage) {
					perf.NetUsage = 0.0
				}
				if math.IsNaN(perf.DiskTotalReadLatency) {
					perf.DiskTotalReadLatency = 0.0
				}
				if math.IsNaN(perf.DiskTotalWriteLatency) {
					perf.DiskTotalWriteLatency = 0.0
				}
				if math.IsNaN(perf.DiskWrite) {
					perf.DiskWrite = 0.0
				}
				if math.IsNaN(perf.DiskRead) {
					perf.DiskRead = 0.0
				}

				perf.ID = vm_p.ID //Id виртуалки в метрики

				vm_p.CPUBilling = vm_p.CPUBilling + float64(perf.CPUUsage)*float64(CpuMhz)*float64(vm_p.VMNumCores)/180.0 //утилизация процессора частота в мегагерцах умноженная процент утилизации и промежуток времени 20 секунд (процент в виде целого умноженого на 100)
				vm_p.MEMBilling = vm_p.MEMBilling + float64(perf.MEMUsage)*vm_p.VMMemorySizeGB*1024.0/180.0               // утилизация памяти мегабайт часах

				if vm_p.CPUUsageMaximum < perf.CPUUsage {
					vm_p.CPUUsageMaximum = perf.CPUUsage
					vm_p.CPUUsageMaximumTime = perf.Start
				}
				if vm_p.CPUUsageMinimum > perf.CPUUsage {
					vm_p.CPUUsageMinimum = perf.CPUUsage
					vm_p.CPUUsageMinimumTime = perf.Start
				}
				vm_p.CPUUsageAverage = vm_p.CPUUsageAverage + perf.CPUUsage

				if vm_p.MEMUsageMaximum < perf.MEMUsage {
					vm_p.MEMUsageMaximum = perf.MEMUsage
					vm_p.MEMUsageMaximumTime = perf.Start
				}
				if vm_p.MEMUsageMinimum > perf.MEMUsage {
					vm_p.MEMUsageMinimum = perf.MEMUsage
					vm_p.MEMUsageMinimumTime = perf.Start
				}
				vm_p.MEMUsageAverage = vm_p.MEMUsageAverage + perf.MEMUsage

				if vm_p.NetUsageMaximum < perf.NetUsage {
					vm_p.NetUsageMaximum = perf.NetUsage
					vm_p.NetUsageMaximumTime = perf.Start
				}
				if vm_p.NetUsageMinimum > perf.NetUsage {
					vm_p.NetUsageMinimum = perf.NetUsage
					vm_p.NetUsageMinimumTime = perf.Start
				}
				vm_p.NetUsageAverage = vm_p.NetUsageAverage + perf.NetUsage

				if vm_p.DiskUsageMaximum < perf.DiskUsage {
					vm_p.DiskUsageMaximum = perf.DiskUsage
					vm_p.DiskUsageMaximumTime = perf.Start
				}
				if vm_p.DiskUsageMinimum > perf.DiskUsage {
					vm_p.DiskUsageMinimum = perf.DiskUsage
					vm_p.DiskUsageMinimumTime = perf.Start
				}
				vm_p.DiskUsageAverage = vm_p.DiskUsageAverage + perf.DiskUsage

				if vm_p.DiskReadMaximum < perf.DiskRead {
					vm_p.DiskReadMaximum = perf.DiskRead
					vm_p.DiskReadMaximumTime = perf.Start
				}
				if vm_p.DiskReadMinimum > perf.DiskRead {
					vm_p.DiskReadMinimum = perf.DiskRead
					vm_p.DiskReadMinimumTime = perf.Start
				}
				vm_p.DiskReadAverage = vm_p.DiskReadAverage + perf.DiskRead

				if vm_p.DiskWriteMaximum < perf.DiskWrite {
					vm_p.DiskWriteMaximum = perf.DiskWrite
					vm_p.DiskWriteMaximumTime = perf.Start
				}
				if vm_p.DiskWriteMinimum > perf.DiskWrite {
					vm_p.DiskWriteMinimum = perf.DiskWrite
					vm_p.DiskWriteMinimumTime = perf.Start
				}
				vm_p.DiskWriteAverage = vm_p.DiskWriteAverage + perf.DiskWrite

				if vm_p.DiskTotalReadLatencyMaximum < perf.DiskTotalReadLatency {
					vm_p.DiskTotalReadLatencyMaximum = perf.DiskTotalReadLatency
					vm_p.DiskTotalReadLatencyMaximumTime = perf.Start
				}
				if vm_p.DiskTotalReadLatencyMinimum > perf.DiskTotalReadLatency {
					vm_p.DiskTotalReadLatencyMinimum = perf.DiskTotalReadLatency
					vm_p.DiskTotalReadLatencyMinimumTime = perf.Start
				}
				vm_p.DiskTotalReadLatencyAverage = vm_p.DiskTotalReadLatencyAverage + perf.DiskTotalReadLatency

				if vm_p.DiskTotalWriteLatencyMaximum < perf.DiskTotalWriteLatency {
					vm_p.DiskTotalWriteLatencyMaximum = perf.DiskTotalWriteLatency
					vm_p.DiskTotalWriteLatencyMaximumTime = perf.Start
				}
				if vm_p.DiskTotalWriteLatencyMinimum > perf.DiskTotalWriteLatency {
					vm_p.DiskTotalWriteLatencyMinimum = perf.DiskTotalWriteLatency
					vm_p.DiskTotalWriteLatencyMinimumTime = perf.Start
				}
				vm_p.DiskTotalWriteLatencyAverage = vm_p.DiskTotalWriteLatencyAverage + perf.DiskTotalWriteLatency

				vm_p.Perf = append(vm_p.Perf, perf) //добавить метрики
				vm_p.PerfCount++

			} else {
				CpuMhz := 0.0
				pc := property.DefaultCollector(c.Client)

				var refs []types.ManagedObjectReference

				refs = append(refs, metric.Entity)

				var vmt []mo.VirtualMachine
				//	                        err = pc.Retrieve(ctx, refs, []string{"name", "config", "summary","hardware","network", "datastore", "storage","guest", "guest.ipAddress", "config.extraConfig", "config.tools"}, &vmt)       //добавить  , "tags"
				err = pc.Retrieve(ctx, refs, nil, &vmt)
				if err != nil {
					fmt.Println("Поиск свойств всех виртуальных машин", err)
					infra.VM.Unlock()
					return err
				}

				for _, vm := range vmt {
					runtime.Gosched()
					if vm.Summary.Runtime.PowerState == "poweredOff" {
						continue
					}
					if strings.Contains(vm.Summary.Config.GuestId, "windows") {
						if !strings.Contains(vm.Summary.Config.GuestId, "Server") {
							continue
						}
					}
					notCluster := "не в кластере"
					clname := notCluster
					dcname := "нет данных"
					//					vcname := "нет данных"
					vcname := vc.Hostname
					if href := vm.Summary.Runtime.Host; href != nil {
						if hi, ok := infra.HSList[vc.Hostname+"/"+href.Value]; ok { //рассмотреть вариант с добавлением в ключ имени vcenter
							//                                  	if hi, ok := infra.HSList[vc_ind+"/"+*href]; ok { //как вариант так
							clname = hi.Cluster
							if clname == "" {
								clname = hi.Name //если это одиночный хост
							}
							dcname = hi.Datacenter
							vcname = hi.Vcenter
							CpuMhz = float64(hi.CPUMhz)
						}
					}

					Id_VM := int64(len(infra.VMList))

					var cl *Cluster = nil

					if clname != notCluster {
						cl, err = infra.getCluster(clname)
					}

					vmp := VMInfo{ID: Id_VM, Vcenter: vcname, Datacenter: dcname, Cluster: clname}

					vmp.CPUUsageMinimum = initFloat64min
					vmp.MEMUsageMinimum = initFloat64min
					vmp.DiskUsageMinimum = initFloat64min
					vmp.DiskTotalReadLatencyMinimum = initFloat64min
					vmp.DiskTotalWriteLatencyMinimum = initFloat64min
					vmp.DiskWriteMinimum = initFloat64min
					vmp.DiskReadMinimum = initFloat64min
					vmp.NetUsageMinimum = initFloat64min

					vmp.importData(vm, infra)
					vmp.KeyName = vcname + "/" + vm.Reference().Value
					if cl != nil {
						cl.VM = append(cl.VM, &vmp)
						cl.VMCount++
					}

					infra.VMList[vcname+"/"+vm.Reference().Value] = &vmp
					//                                  fmt.Printf("%-40s %-40s\n",vmp.vm.Summary.Config.Name, vmp.cluster)

					fmt.Printf("Добавлена виртуальная машина %s %s %s %s %s\n", name, vmp.VMName, vmp.Cluster, vmp.Datacenter, vmp.Vcenter)

					perf := VMPerf{CPUUsage: 0.0,
						MEMUsage:              0.0,
						DiskUsage:             0.0,
						DiskTotalReadLatency:  0.0,
						DiskTotalWriteLatency: 0.0,
						DiskWrite:             0.0,
						DiskRead:              0.0,
						NetUsage:              0.0}
					if len(metric.SampleInfo) > 0 {
						perf.Start = metric.SampleInfo[0].Timestamp
					} // время среза метрик и интервал по первой записи
					if len(vmp.Perf) == 0 {
						vmp.Perf = make([]VMPerf, 0)
					}

					for _, v := range metric.Value {
						runtime.Gosched()
						if len(v.Value) > 0 {
							//fmt.Printf("2 - %s\n", v.Name)
							switch v.Name {
							case "cpu.usage.average":
								if v.Instance == "" {
									perf.CPUUsage = float64(v.Value[0]) / 100.0
								}
							case "mem.usage.average":
								perf.MEMUsage = float64(v.Value[0]) / 100.0
							case "disk.usage.average":
								perf.DiskUsage = float64(v.Value[0]) / 1024.0
							case "net.usage.average":
								perf.NetUsage = float64(v.Value[0]) / 1024.0
							case "virtualDisk.totalReadLatency.average":
								perf.DiskTotalReadLatency = float64(v.Value[0]) //ms
							case "virtualDisk.totalWriteLatency.average":
								perf.DiskTotalWriteLatency = float64(v.Value[0]) //ms
							case "virtualDisk.write.average":
								perf.DiskWrite = float64(v.Value[0]) / 1024.0
							case "virtualDisk.read.average":
								perf.DiskRead = float64(v.Value[0]) / 1024.0
							}
						}
					}
					if math.IsNaN(perf.CPUUsage) {
						perf.CPUUsage = 0.0
					}
					if math.IsNaN(perf.MEMUsage) {
						perf.MEMUsage = 0.0
					}
					if math.IsNaN(perf.DiskUsage) {
						perf.DiskUsage = 0.0
					}
					if math.IsNaN(perf.NetUsage) {
						perf.NetUsage = 0.0
					}
					if math.IsNaN(perf.DiskTotalReadLatency) {
						perf.DiskTotalReadLatency = 0.0
					}
					if math.IsNaN(perf.DiskTotalWriteLatency) {
						perf.DiskTotalWriteLatency = 0.0
					}
					if math.IsNaN(perf.DiskWrite) {
						perf.DiskWrite = 0.0
					}
					if math.IsNaN(perf.DiskRead) {
						perf.DiskRead = 0.0
					}

					perf.ID = vmp.ID                                                                                       //Id виртуалки в метрики
					vmp.CPUBilling = vmp.CPUBilling + float64(perf.CPUUsage)*float64(CpuMhz)*float64(vmp.VMNumCores)/180.0 //утилизация процессора частота в мегагерцах умноженная процент утилизации и промежуток времени 20 секунд (процент в виде целого умноженого на 100)
					vmp.MEMBilling = vmp.MEMBilling + float64(perf.MEMUsage)*vmp.VMMemorySizeGB*1024.0/180.0               // утилизация памяти мегабайт часах

					if vmp.CPUUsageMaximum < perf.CPUUsage {
						vmp.CPUUsageMaximum = perf.CPUUsage
						vmp.CPUUsageMaximumTime = perf.Start
					}
					if vmp.CPUUsageMinimum > perf.CPUUsage {
						vmp.CPUUsageMinimum = perf.CPUUsage
						vmp.CPUUsageMinimumTime = perf.Start
					}
					vmp.CPUUsageAverage = vmp.CPUUsageAverage + perf.CPUUsage

					if vmp.MEMUsageMaximum < perf.MEMUsage {
						vmp.MEMUsageMaximum = perf.MEMUsage
						vmp.MEMUsageMaximumTime = perf.Start
					}
					if vmp.MEMUsageMinimum > perf.MEMUsage {
						vmp.MEMUsageMinimum = perf.MEMUsage
						vmp.MEMUsageMinimumTime = perf.Start
					}
					vmp.MEMUsageAverage = vmp.MEMUsageAverage + perf.MEMUsage

					if vmp.NetUsageMaximum < perf.NetUsage {
						vmp.NetUsageMaximum = perf.NetUsage
						vmp.NetUsageMaximumTime = perf.Start
					}
					if vmp.NetUsageMinimum > perf.NetUsage {
						vmp.NetUsageMinimum = perf.NetUsage
						vmp.NetUsageMinimumTime = perf.Start
					}
					vmp.NetUsageAverage = vmp.NetUsageAverage + perf.NetUsage

					if vmp.DiskUsageMaximum < perf.DiskUsage {
						vmp.DiskUsageMaximum = perf.DiskUsage
						vmp.DiskUsageMaximumTime = perf.Start
					}
					if vmp.DiskUsageMinimum > perf.DiskUsage {
						vmp.DiskUsageMinimum = perf.DiskUsage
						vmp.DiskUsageMinimumTime = perf.Start
					}
					vmp.DiskUsageAverage = vmp.DiskUsageAverage + perf.DiskUsage

					if vmp.DiskReadMaximum < perf.DiskRead {
						vmp.DiskReadMaximum = perf.DiskRead
						vmp.DiskReadMaximumTime = perf.Start
					}
					if vmp.DiskReadMinimum > perf.DiskRead {
						vmp.DiskReadMinimum = perf.DiskRead
						vmp.DiskReadMinimumTime = perf.Start
					}
					vmp.DiskReadAverage = vmp.DiskReadAverage + perf.DiskRead

					if vmp.DiskWriteMaximum < perf.DiskWrite {
						vmp.DiskWriteMaximum = perf.DiskWrite
						vmp.DiskWriteMaximumTime = perf.Start
					}
					if vmp.DiskWriteMinimum > perf.DiskWrite {
						vmp.DiskWriteMinimum = perf.DiskWrite
						vmp.DiskWriteMinimumTime = perf.Start
					}
					vmp.DiskWriteAverage = vmp.DiskWriteAverage + perf.DiskWrite

					if vmp.DiskTotalReadLatencyMaximum < perf.DiskTotalReadLatency {
						vmp.DiskTotalReadLatencyMaximum = perf.DiskTotalReadLatency
						vmp.DiskTotalReadLatencyMaximumTime = perf.Start
					}
					if vmp.DiskTotalReadLatencyMinimum > perf.DiskTotalReadLatency {
						vmp.DiskTotalReadLatencyMinimum = perf.DiskTotalReadLatency
						vmp.DiskTotalReadLatencyMinimumTime = perf.Start
					}
					vmp.DiskTotalReadLatencyAverage = vmp.DiskTotalReadLatencyAverage + perf.DiskTotalReadLatency

					if vmp.DiskTotalWriteLatencyMaximum < perf.DiskTotalWriteLatency {
						vmp.DiskTotalWriteLatencyMaximum = perf.DiskTotalWriteLatency
						vmp.DiskTotalWriteLatencyMaximumTime = perf.Start
					}
					if vmp.DiskTotalWriteLatencyMinimum > perf.DiskTotalWriteLatency {
						vmp.DiskTotalWriteLatencyMinimum = perf.DiskTotalWriteLatency
						vmp.DiskTotalWriteLatencyMinimumTime = perf.Start
					}
					vmp.DiskTotalWriteLatencyAverage = vmp.DiskTotalWriteLatencyAverage + perf.DiskTotalWriteLatency

					vmp.Perf = append(vmp.Perf, perf) //добавить метрики
					vmp.PerfCount++
				}
			}
		}
		infra.VM.Unlock()

	}
	return nil
}

//ListVMPerf вывод списка виртуальных машин и метрик
func (infra *VMInfrastructure) ListVMPerf() error {

	if infra == nil {
		return errors.New(vminfraNotInitialised)
	}

	fmt.Println("Счетчики производительности VM")

	infra.VM.RLock()
	defer infra.VM.RUnlock()

	for _, point := range infra.VMList {
		runtime.Gosched()
		if !point.VMPowerState {
			continue
		} //для выключенных машин нет информации по загрузке
		fmt.Printf("Id: %10d Cluster: %-30s Name: %-30s CPU: %4d Memory: %4d\n", point.ID, point.Cluster, point.VMName, point.VMNumCPU, point.VMMemorySizeGB)
		for _, perf := range point.Perf {
			ts := perf.Start.Format("2006-01-02 15:04:05")
			fmt.Printf("%-20s CPU  %6.2f RAM  %6.2f\n", ts, float64(perf.CPUUsage), float64(perf.MEMUsage))
		}
	}
	return nil
}

//LoadDCHSCLInfo - функция загрузки данных по хостам и кластерам
func (infra *VMInfrastructure) LoadDCHSCLInfo() error {

	if infra == nil {
		return errors.New(vminfraNotInitialised)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Id_HS := int64(0)

	for _, vc := range infra.VCList {
		c, err := vc.connect()
		if err != nil {
			log.Printf("Ошибка подключения: %s\n", err.Error())
			continue
		}
		defer c.Logout(ctx)

		f := find.NewFinder(c.Client, true)

		dclist, err := f.DatacenterList(ctx, "*")
		if err != nil {
			fmt.Println("Список датацентров", err)
			return err
		}

		for _, dc := range dclist {

			dcname, err := dc.ObjectName(ctx)
			if err != nil {
				fmt.Println("Запрос имени датацентра", err)
				return err
			}

			f.SetDatacenter(dc)

			cluster_list, err := f.ClusterComputeResourceList(ctx, "*")
			if err != nil {
				log.Println("Список кластеров", err)
				return err
			}

			//object.ClusterComputeResource
			for _, cluster := range cluster_list {
				clname, err := cluster.ObjectName(ctx)
				if err != nil {
					log.Println("Запрос имени кластера", err)
					return err
				}
				//                            fmt.Printf("\t%s\n", clname)
				cl := Cluster{Date: time.Now().Local(), Vcenter: vc.Hostname, Datacenter: dcname, Name: clname}

				cl.Storage = make([]*Datastore, 0)
				infra.CLList = append(infra.CLList, &cl)

				dstores, err := cluster.Datastores(ctx)
				if err != nil {
					log.Println("Запрос списка хранилищ", err)
				}
				ids := 0

				for _, dstor := range dstores {
					var dss []mo.Datastore
					//							err = c.RetrieveOne(ctx, dstor.Reference(), []string{"summary"}, &dss)
					err = c.RetrieveOne(ctx, dstor.Reference(), nil, &dss)
					if err != nil {
						log.Println("Ошибка получения информации о хранилище: ", err)
					}
					for _, ds := range dss {
						lun := Datastore{ID: int64(ids), StorageName: ds.Summary.Name,
							CapacityGb:  float64(ds.Summary.Capacity) / (1024.0 * 1024.0 * 1024.0),
							FreeSpaceGb: float64(ds.Summary.FreeSpace) / (1024.0 * 1024.0 * 1024.0),
							PercentUsed: 100.0 * (float64(ds.Summary.Capacity) - float64(ds.Summary.FreeSpace)) / float64(ds.Summary.Capacity)}
						for _, vmd := range ds.Vm {
							vmdNew := vmd
							lun.vmnames = append(lun.vmnames, vmdNew)
						}
						cl.Storage = append(cl.Storage, &lun)
						ref := dstor.Reference()
						infra.DSList[cl.Vcenter+"/"+ref.Value] = &lun
						cl.StorageCount++
						ids++
						cl.AllCapacityGb = cl.AllCapacityGb + lun.CapacityGb
						cl.AllFreeSpaceGb = cl.AllFreeSpaceGb + lun.FreeSpaceGb
					}
				}
				if cl.StorageCount > 0 {
					cl.AllPercentUsed = 100.0 * (cl.AllCapacityGb - cl.AllFreeSpaceGb) / cl.AllCapacityGb
				}

				hosts, err := cluster.Hosts(ctx)

				if err != nil {
					log.Println("Запрос списка хостов", err)
					return err
				}

				for _, host := range hosts {
					var hss []mo.HostSystem

					err = c.RetrieveOne(ctx, host.Reference(), []string{"summary", "parent", "hardware", "network", "runtime", "config", "datastore"}, &hss)
					if err != nil {
						log.Println("Ошибка получения информации по хостам ", err)
					}
					runtime.Gosched()
					//Блокируем
					infra.HS.Lock()

					for _, hs := range hss {
						if hs.Summary.Config.Name == "s-msk-esxi-09.global.bcs" {
							_ = hs.Summary.Config.Name
						}
						runtime.Gosched()
						hi := HostInfo{ID: Id_HS, Vcenter: vc.Hostname, Datacenter: dcname, Cluster: clname}

						hi.hsClearPerfomance()
						hi.importData(hs)
						hi.HostMaxVirtualDiskCapacity = cl.AllCapacityGb
						Id_HS++
						infra.HSList[vc.Hostname+"/"+hs.Summary.Host.Value] = &hi
						hi.KeyName = vc.Hostname + "/" + hs.Summary.Host.Value
						cl.Hosts = append(cl.Hosts, &hi)
					}
					//снимаем блокировку
					infra.HS.Unlock()
				}
			}
			compute_list, err := f.ComputeResourceList(ctx, "*")
			if err != nil {
				log.Println("Список Compute", err)
				return err
			}

			for _, compute := range compute_list {
				clname, err := compute.ObjectName(ctx)
				if err != nil {
					log.Println("Запрос имени кластера", err)
					return err
				}
				refType := compute.Reference().Type
				if refType == "ClusterComputeResource" {
					continue
				}

				runtime.Gosched()
				cl := Cluster{Date: time.Now().Local(), Vcenter: vc.Hostname, Datacenter: dcname, Name: clname}
				cl.Storage = make([]*Datastore, 0)
				infra.CLList = append(infra.CLList, &cl)

				dstores, err := compute.Datastores(ctx)
				if err != nil {
					log.Println("Запрос списка хранилищ", err)
				}
				ids := 0
				for _, dstor := range dstores {

					var dss []mo.Datastore
					//							err = c.RetrieveOne(ctx, dstor.Reference(), []string{"summary"}, &dss)
					err = c.RetrieveOne(ctx, dstor.Reference(), nil, &dss)
					if err != nil {
						log.Println("Ошибка при получении списка хранилищ ", err)
					}

					for _, ds := range dss {
						lun := Datastore{ID: int64(ids), StorageName: ds.Summary.Name,
							CapacityGb:  float64(ds.Summary.Capacity) / (1024.0 * 1024.0 * 1024.0),
							FreeSpaceGb: float64(ds.Summary.FreeSpace) / (1024.0 * 1024.0 * 1024.0),
							PercentUsed: 100.0 * (float64(ds.Summary.Capacity) - float64(ds.Summary.FreeSpace)) / float64(ds.Summary.Capacity)}

						for _, vmd := range ds.Vm {
							vmdNew := vmd
							lun.vmnames = append(lun.vmnames, vmdNew)
						}
						cl.Storage = append(cl.Storage, &lun)
						infra.DSList[cl.Vcenter+"/"+dstor.Reference().Value] = &lun
						cl.StorageCount++
						ids++
						cl.AllCapacityGb = cl.AllCapacityGb + lun.CapacityGb
						cl.AllFreeSpaceGb = cl.AllFreeSpaceGb + lun.FreeSpaceGb
						ids++
					}
					if cl.StorageCount > 0 {
						cl.AllPercentUsed = 100.0 * (cl.AllCapacityGb - cl.AllFreeSpaceGb) / cl.AllCapacityGb
					}

				}

				hosts, err := compute.Hosts(ctx)

				if err != nil {
					log.Println("Запрос списка хостов", err)
					return err
				}

				for _, host := range hosts {
					var hss []mo.HostSystem
					err = c.RetrieveOne(ctx, host.Reference(), []string{"summary", "parent", "hardware", "network", "runtime", "config", "datastore"}, &hss)
					if err != nil {
						log.Println("Ошибка получения информации по хостам ", err)
					}
					//блокируем
					infra.HS.Lock()
					for _, hs := range hss {
						if hs.Parent.Type == "ClusterComputeResource" {
							continue
						}
						if cl.Name == "" {
							cl.Name = hs.Summary.Config.Name //если поле кластер пустое, считаем это отдельным хостом но учитываем как кластер
						}
						hi := HostInfo{ID: Id_HS, Vcenter: vc.Hostname, Datacenter: dcname, Cluster: cl.Name} // возможно Cluster лучше указать ""
						hi.hsClearPerfomance()
						hi.importData(hs)
						hi.HostMaxVirtualDiskCapacity = cl.AllCapacityGb
						//
						Id_HS++
						infra.HSList[vc.Hostname+"/"+hs.Summary.Host.Value] = &hi //возможно нужно отработать схему с учетом добавления в ключ имени vcenter
						hi.KeyName = vc.Hostname + "/" + hs.Summary.Host.Value
						cl.Hosts = append(cl.Hosts, &hi)

					}
					//снимаем блокировку
					infra.HS.Unlock()
				}

			}
		}
	}
	return nil
}
