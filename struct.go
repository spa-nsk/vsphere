package vsphere

import (
	"sync"
	"time"

	"github.com/vmware/govmomi/vim25/types"
)

//VCenter  структура описывающая управляющий vcenter
//
type VCenter struct {
	Hostname string `form:"Hostname"`
	Username string `form:"Username"`
	Password string `form:"Password"`
}

//VMInfrastructure основная структура для хранения всех данных собранных из одного или нескольких Vcenter
//
type VMInfrastructure struct {
	VC          sync.RWMutex          //доступ к VCList
	CL          sync.RWMutex          //доступ к CLList
	VM          sync.RWMutex          //доступ к VMList
	HS          sync.RWMutex          //доступ к HSList
	DS          sync.RWMutex          //доступ к DSList
	IN          sync.RWMutex          //доступ к Initialized
	Initialized bool                  //загружена информация по кластерам, хостам, виртуальным машинам
	VCList      []VCenter             //срез с данными по обрабатываемым VCenter
	CLList      []*Cluster            //срез с с указателями на данные по кластерам обрабатываемых VCenter
	VMList      map[string]*VMInfo    //данные по структурам виртуальных машин
	HSList      map[string]*HostInfo  //данные по хостам обрабатываемых кластеров
	DSList      map[string]*Datastore //данные по хранилищам обрабатываемых кластеров
}

//VMPerf описание данных по метрикам виртуальных машин
//
type VMPerf struct {
	ID                    int64     `form:"ID"`                    //идентификатор метрики
	Start                 time.Time `form:"Start"`                 //время фиксации метрики
	CPUUsage              float64   `form:"CPUUsage"`              //загрузка процессоров виртуальной машины в процентах
	MEMUsage              float64   `form:"MEMUsage"`              //использование оперативной памяти виртуальной машины в процентах
	DiskUsage             float64   `form:"DiskUsage"`             //скорость диска kbps
	DiskTotalReadLatency  float64   `form:"DiskTotalReadLatency"`  //Среднее время чтения с виртуального диска
	DiskTotalWriteLatency float64   `form:"DiskTotalWriteLatency"` //Среднее время записи на виртуальный диск
	DiskWrite             float64   `form:"DiskWrite"`             //Скорость записи данных на виртуальный диск kbps
	DiskRead              float64   `form:"DiskRead"`              //Скорость чтения данных с виртуального диска kbps
	NetUsage              float64   `form:"NetUsage"`              //Использование сети (комбинированные скорости передачи и скорости приема) в течение интервала kbps
}

//HSPerf описание данных по метрикам хостов
//
type HSPerf struct {
	ID                    int64     `form:"ID"`                    //идентификатор метрики
	Start                 time.Time `form:"Start"`                 //время фиксации метрики
	CPUUsage              float64   `form:"CPUUsage"`              //Загрузка хоста в процентах
	MEMUsage              float64   `form:"MEMUsage"`              //использование оперативной памяти хостом
	DiskUsage             float64   `form:"DiskUsage"`             //скорость диска kbps
	DiskTotalReadLatency  float64   `form:"DiskTotalReadLatency"`  //Среднее время чтения с виртуального диска
	DiskTotalWriteLatency float64   `form:"DiskTotalWriteLatency"` //Среднее время записи на виртуальный диск
	DiskWrite             float64   `form:"DiskWrite"`             //Скорость записи данных на виртуальный диск kbps
	DiskRead              float64   `form:"DiskRead"`              //Скорость чтения данных с виртуального диска kbps
	NetUsage              float64   `form:"NetUsage"`              //Использование сети (комбинированные скорости передачи и скорости приема) в течение интервала kbps
}

//CLPerf описание данных по метрикам кластера
//
type CLPerf struct {
	ID                    int64     `form:"ID"`                    //идентификатор метрики
	Start                 time.Time `form:"Start"`                 //ремя фиксации метрики
	CPUUsage              float64   `form:"CPUUsage"`              //агрузка кластера в процентах
	MEMUsage              float64   `form:"MEMUsage"`              //спользование оперативной памяти кластером
	DiskUsage             float64   `form:"DiskUsage"`             //скорость диска kbps
	DiskTotalReadLatency  float64   `form:"DiskTotalReadLatency"`  //Среднее время чтения с виртуального диска
	DiskTotalWriteLatency float64   `form:"DiskTotalWriteLatency"` //Среднее время записи на виртуальный диск
	DiskWrite             float64   `form:"DiskWrite"`             //Скорость записи данных на виртуальный диск kbps
	DiskRead              float64   `form:"DiskRead"`              //Скорость чтения данных с виртуального диска kbps
	NetUsage              float64   `form:"NetUsage"`              //Использование сети (комбинированные скорости передачи и скорости приема) в течение интервала kbps
}

//GuestDiskInfo информация о диске в системе
//
type GuestDiskInfo struct {
	DiskPath    string  //путь диска
	CapacityGb  float64 //емкость
	FreeSpaceGb float64 //свободное место в gb
	PercentUsed float64 //занято в процентах
}

//VMUsageOnDatastore описание данных хранилища
//
type VMUsageOnDatastore struct {
	ID                     int64   `form:"ID"`                     //
	Datastore              string  `form:"Datastore"`              //
	StorageName            string  `form:"StorageName"`            //
	CommittedGb            float64 `form:"Committed"`              //
	UncommittedGb          float64 `form:"Uncommitted"`            //
	UnsharedGb             float64 `form:"Unshared"`               //
	PercentAllocateStorage float64 `form:"PercentAllocateStorage"` //процент выделения места под виртуальную машину на конкретном хранилище
}

//VMInfo описание виртуальной машины
//
type VMInfo struct {
	ID                               int64                `form:"ID"`                                 //
	Vcenter                          string               `form:"Vcenter"`                            //
	Datacenter                       string               `form:"Datacenter"`                         //
	Cluster                          string               `form:"Cluster"`                            //
	KeyName                          string               `form:"KeyName"`                            //
	VMName                           string               `form:"VMName"`                             //
	VMGuestFullName                  string               `form:"VMGuestFullName"`                    //
	VMHostName                       string               `form:"VMHostName"`                         //
	VMPathName                       string               `form:"VMPathName"`                         //
	VMPowerState                     bool                 `form:"VMPowerState"`                       //true - VM включена
	VMUUID                           string               `form:"VMUUID"`                             //
	VMGuestID                        string               `form:"VMGuestID"`                          //
	VMAnnotation                     string               `form:"VMAnnotation"`                       //
	VMNumCPU                         int32                `form:"VMNumCPU"`                           //
	VMNumCores                       int32                `form:"VMNumCores"`                         //
	VMMemorySizeGB                   float64              `form:"VMMemorySizeGB"`                     //
	VMIpAddress                      string               `form:"VMIpAddress"`                        //
	VMNumEthernetCards               int32                `form:"VMNumEthernetCards"`                 //
	VMNumVirtualDisks                int32                `form:"VMNumVirtualDisks"`                  //
	VMDiskCommittedGB                float64              `form:"VMDiskCommittedGB"`                  //
	VMDiskUncommittedGB              float64              `form:"VMDiskUncommittedGB"`                //
	VMCapacityGb                     float64              `form:"VMCapacityGb"`                       //
	VMFreeSpaceGb                    float64              `form:"VMFreeSpaceGb"`                      //
	VMPercentUsed                    float64              `form:"VMPercentUsed"`                      //
	CPUUsageMaximum                  float64              `form:"CPUUsageMaximum"`                    //
	CPUUsageMaximumTime              time.Time            `form:"CPUUsageMaximumTime"`                //
	CPUUsageAverage                  float64              `form:"CPUUsageAverage"`                    //
	CPUUsageMinimum                  float64              `form:"CPUUsageMinimum"`                    //
	CPUUsageMinimumTime              time.Time            `form:"CPUUsageMinimumTime"`                //
	MEMUsageMaximum                  float64              `form:"MEMUsageMaximum"`                    //
	MEMUsageMaximumTime              time.Time            `form:"MEMUsageMaximumTime"`                //
	MEMUsageAverage                  float64              `form:"MEMUsage_average"`                   //
	MEMUsageMinimum                  float64              `form:"MEMUsage_minimum"`                   //
	MEMUsageMinimumTime              time.Time            `form:"MEMUsageMinimum_time"`               //
	DiskUsageMaximum                 float64              `form:"DiskUsageMaximum"`                   //
	DiskUsageMaximumTime             time.Time            `form:"DiskUsageMaximumTime"`               //
	DiskUsageAverage                 float64              `form:"DiskUsageAverage"`                   //
	DiskUsageMinimum                 float64              `form:"DiskUsageMinimum"`                   //
	DiskUsageMinimumTime             time.Time            `form:"DiskUsageMinimumTime"`               //
	DiskTotalReadLatencyMaximum      float64              `form:"DiskTotalReadLatencyMaximum"`        //
	DiskTotalReadLatencyMaximumTime  time.Time            `form:"Disk_totalReadLatency_maximum_time"` //
	DiskTotalReadLatencyAverage      float64              `form:"DiskTotalReadLatencyAverage"`        //
	DiskTotalReadLatencyMinimum      float64              `form:"DiskTotalReadLatencyMinimum"`        //
	DiskTotalReadLatencyMinimumTime  time.Time            `form:"DiskTotalReadLatencyMinimumTime"`    //
	DiskTotalWriteLatencyMaximum     float64              `form:"DiskTotalWriteLatencyMaximum"`       //
	DiskTotalWriteLatencyMaximumTime time.Time            `form:"DiskTotalWriteLatencyMaximumTime"`   //
	DiskTotalWriteLatencyAverage     float64              `form:"DiskTotalWriteLatencyAverage"`       //
	DiskTotalWriteLatencyMinimum     float64              `form:"DiskTotalWriteLatencyMinimum"`       //
	DiskTotalWriteLatencyMinimumTime time.Time            `form:"DiskTotalWriteLatencyMinimumTime"`   //
	DiskWriteMaximum                 float64              `form:"DiskWriteMaximum"`                   //
	DiskWriteMaximumTime             time.Time            `form:"DiskWriteMaximumTime"`               //
	DiskWriteAverage                 float64              `form:"DiskWriteAverage"`                   //
	DiskWriteMinimum                 float64              `form:"DiskWriteMinimum"`                   //
	DiskWriteMinimumTime             time.Time            `form:"DiskWriteMinimumTime"`               //
	DiskReadMaximum                  float64              `form:"DiskReadMaximum"`                    //
	DiskReadMaximumTime              time.Time            `form:"DiskReadMaximumTime"`                //
	DiskReadAverage                  float64              `form:"DiskReadAverage"`                    //
	DiskReadMinimum                  float64              `form:"DiskReadMinimum"`                    //
	DiskReadMinimumTime              time.Time            `form:"DiskReadMinimumTime"`                //
	NetUsageMaximum                  float64              `form:"NetUsageMaximum"`                    //
	NetUsageMaximumTime              time.Time            `form:"NetUsageMaximumTime"`                //
	NetUsageAverage                  float64              `form:"NetUsageAverage"`                    //
	NetUsageMinimum                  float64              `form:"NetUsageMinimum"`                    //
	NetUsageMinimumTime              time.Time            `form:"NetUsageMinimumTime"`                //
	CPUBilling                       float64              `form:"CPUBilling"`                         // в мегагерц часах
	MEMBilling                       float64              `form:"MEMBilling"`                         // в мегабайт часах
	DiskBilling                      float64              `form:"DiskBilling"`                        // по занимаемому месту в гигабайт часах
	NetBilling                       float64              `form:"NetBilling"`                         // в мегабайт часах
	VMDisk                           []GuestDiskInfo      `form:"VMDDisk"`                            //
	UsageDatastore                   []VMUsageOnDatastore `form:"UsageDatastore"`                     //
	Perf                             []VMPerf             `form:"Perf"`                               //
	PerfCount                        int64                `form:"PerfCount"`                          //
}

//Datastore описание харнилища в кластере
//
type Datastore struct {
	ID          int64                          `form:"ID"`          //
	StorageName string                         `form:"StorageName"` //
	CapacityGb  float64                        `form:"CapacityGb"`  //
	FreeSpaceGb float64                        `form:"FreeSpaceGb"` //
	PercentUsed float64                        `form:"PercentUsed"` //
	VM          []*VMInfo                      `form:"VM"`          //
	VMCount     int64                          `form:VMCount`       //
	vmnames     []types.ManagedObjectReference //
}

//Cluster описание кластера
//
type Cluster struct {
	ID                               int64        `form:"ID"`                               //
	Date                             time.Time    `form:"Date"`                             //
	Vcenter                          string       `form:"Vcenter"`                          //
	Datacenter                       string       `form:"Datacenter"`                       //
	Name                             string       `form:"Name"`                             //
	CPUMhzAvg                        int32        `form:"CPUMhzAvg"`                        //
	AllNumCPUPkgs                    int16        `form:"AllNumCPUPkgs"`                    //
	AllNumCPUCores                   int16        `form:"AllNumCPUCores"`                   //
	AllNumCPUThreads                 int16        `form:"AllNumCPUThreads"`                 //
	AllMemorySizeGB                  float64      `form:"AllMemorySizeGB"`                  //
	AllCapacityGb                    float64      `form:"AllCapacityGb"`                    //
	AllFreeSpaceGb                   float64      `form:"AllFreeSpaceGb"`                   //
	AllPercentUsed                   float64      `form:"AllPercentUsed"`                   //
	CPUUsageMaximum                  float64      `form:"CPUUsageMaximum"`                  //
	CPUUsageMaximumTime              time.Time    `form:"CPUUsageMaximumTime"`              //
	CPUUsageAverage                  float64      `form:"CPUUsageAverage"`                  //
	CPUUsageMinimum                  float64      `form:"CPUUsageMinimum"`                  //
	CPUUsageMinimumTime              time.Time    `form:"CPUUsageMinimumTime"`              //
	MEMUsageMaximum                  float64      `form:"MEMUsageMaximum"`                  //
	MEMUsageMaximumTime              time.Time    `form:"MEMUsageMaximumTime"`              //
	MEMUsageAverage                  float64      `form:"MEMUsageAverage"`                  //
	MEMUsageMinimum                  float64      `form:"MEMUsageMinimum"`                  //
	MEMUsageMinimumTime              time.Time    `form:"MEMUsageMinimumTime"`              //
	DiskUsageMaximum                 float64      `form:"DiskUsageMaximum"`                 //
	DiskUsageMaximumTime             time.Time    `form:"DiskUsageMaximumTime"`             //
	DiskUsageAverage                 float64      `form:"DiskUsageAverage"`                 //
	DiskUsageMinimum                 float64      `form:"DiskUsageMinimum"`                 //
	DiskUsageMinimumTime             time.Time    `form:"DiskUsageMinimumTime"`             //
	DiskTotalReadLatencyMaximum      float64      `form:"DiskTotalReadLatencyMaximum"`      //
	DiskTotalReadLatencyMaximumTime  time.Time    `form:"DiskTotalReadLatencyMaximumTime"`  //
	DiskTotalReadLatencyAverage      float64      `form:"DiskTotalReadLatencyAverage"`      //
	DiskTotalReadLatencyMinimum      float64      `form:"DiskTotalReadLatencyMinimum"`      //
	DiskTotalReadLatencyMinimumTime  time.Time    `form:"DiskTotalReadLatencyMinimumTime"`  //
	DiskTotalWriteLatencyMaximum     float64      `form:"DiskTotalWriteLatencyMaximum"`     //
	DiskTotalWriteLatencyMaximumTime time.Time    `form:"DiskTotalWriteLatencyMaximumTime"` //
	DiskTotalWriteLatencyAverage     float64      `form:"DiskTotalWriteLatencyAverage"`     //
	DiskTotalWriteLatencyMinimum     float64      `form:"DiskTotalWriteLatencyMinimum"`     //
	DiskTotalWriteLatencyMinimumTime time.Time    `form:"DiskTotalWriteLatencyMinimumTime"` //
	DiskWriteMaximum                 float64      `form:"DiskWriteMaximum"`                 //
	DiskWriteMaximumTime             time.Time    `form:"DiskWriteMaximumTime"`             //
	DiskWriteAverage                 float64      `form:"DiskWriteAverage"`                 //
	DiskWriteMinimum                 float64      `form:"DiskWriteMinimum"`                 //
	DiskWriteMinimumTime             time.Time    `form:"DiskWriteMinimumTime"`             //
	DiskReadMaximum                  float64      `form:"DiskReadMaximum"`                  //
	DiskReadMaximumTime              time.Time    `form:"DiskReadMaximumTime"`              //
	DiskReadAverage                  float64      `form:"DiskReadAverage"`                  //
	DiskReadMinimum                  float64      `form:"DiskReadMinimum"`                  //
	DiskReadMinimumTime              time.Time    `form:"DiskReadMinimumTime"`              //
	NetUsageMaximum                  float64      `form:"NetUsageMaximum"`                  //
	NetUsageMaximumTime              time.Time    `form:"NetUsageMaximumTime"`              //
	NetUsageAverage                  float64      `form:"NetUsageAverage"`                  //
	NetUsageMinimum                  float64      `form:"NetUsageMinimum"`                  //
	NetUsageMinimumTime              time.Time    `form:"NetUsageMinimumTime"`              //
	CPUBilling                       float64      `form:"CPUBilling"`                       // в мегагерц часах
	DiskBilling                      float64      `form:"DiskBilling"`                      // по занимаемому месту в гигабайт часах
	MEMBilling                       float64      `form:"MEMBilling"`                       // в мегабайт часах
	NetBilling                       float64      `form:"NetBilling"`                       // в мегабайт часах
	Hosts                            []*HostInfo  `form:"Hosts"`                            //
	HostCount                        int64        `form:"HostCount"`                        //
	VM                               []*VMInfo    `form:"VM"`                               //
	VMCount                          int64        `form:"VMCount"`                          //
	Storage                          []*Datastore `form:"Storage"`                          //
	StorageCount                     int64        `form:"StorageCount"`                     //
	Perf                             []CLPerf     `form:"Perf"`                             //
	PerfCount                        int64        `form:"PerfCount"`                        //
}

//HostInfo информация о хосте
//
type HostInfo struct {
	ID                               int64     `form:"ID"`                               //
	Vcenter                          string    `form:"Vcenter"`                          //
	Datacenter                       string    `form:"Datacenter"`                       //
	Cluster                          string    `form:"Cluster"`                          //
	Name                             string    `form:"Name"`                             //
	KeyName                          string    `form:"KeyName"`                          //
	Vendor                           string    `form:"Vendor"`                           //HostSystemInfo
	Model                            string    `form:"Model"`                            //
	SerialNumber                     string    `form:"SerialNumber"`                     //
	EnclosureSerialNumber            string    `form:"EnclosureSerialNumber"`            //
	UUID                             string    `form:"UUID"`                             //
	PowerState                       bool      `form:"PowerState"`                       //true host включен
	InMaintenanceMode                bool      `form:"InMaintenanceMode"`                //
	CPUModel                         string    `form:"CPUModel"`                         //
	CPUMhz                           int32     `form:"CPUMhz"`                           //
	NumCPUPkgs                       int16     `form:"NumCPUPkgs"`                       //
	NumCPUCores                      int16     `form:"NumCPUCores"`                      //
	NumCPUThreads                    int16     `form:"NumCPUThreads"`                    //
	NumNics                          int32     `form:"NumNics"`                          //
	NumHBAs                          int32     `form:"NumHBAs"`                          //
	MemorySizeGB                     float64   `form:"MemorySizeGB"`                     //
	HostMaxVirtualDiskCapacity       float64   `form:"HostMaxVirtualDiskCapacity"`       //
	CPUUsageMaximum                  float64   `form:"CPUUsageMaximum"`                  //
	CPUUsageMaximumTime              time.Time `form:"CPUUsageMaximumTime"`              //
	CPUUsageAverage                  float64   `form:"CPUUsageAverage"`                  //
	CPUUsageMinimum                  float64   `form:"CPUUsageMinimum"`                  //
	CPUUsageMinimumTime              time.Time `form:"CPUUsageMinimumTime"`              //
	MEMUsageMaximum                  float64   `form:"MEMUsageMaximum"`                  //
	MEMUsageMaximumTime              time.Time `form:"MEMUsageMaximumTime"`              //
	MEMUsageAverage                  float64   `form:"MEMUsageAverage"`                  //
	MEMUsageMinimum                  float64   `form:"MEMUsageMinimum"`                  //
	MEMUsageMinimumTime              time.Time `form:"MEMUsageMinimumTime"`              //
	DiskUsageMaximum                 float64   `form:"DiskUsageMaximum"`                 //
	DiskUsageMaximumTime             time.Time `form:"DiskUsageMaximumTime"`             //
	DiskUsageAverage                 float64   `form:"DiskUsageAverage"`                 //
	DiskUsageMinimum                 float64   `form:"DiskUsageMinimum"`                 //
	DiskUsageMinimumTime             time.Time `form:"DiskUsageMinimumTime"`             //
	DiskTotalReadLatencyMaximum      float64   `form:"DiskTotalReadLatencyMaximum"`      //
	DiskTotalReadLatencyMaximumTime  time.Time `form:"DiskTotalReadLatencyMaximumTime"`  //
	DiskTotalReadLatencyAverage      float64   `form:"DiskTotalReadLatencyAverage"`      //
	DiskTotalReadLatencyMinimum      float64   `form:"DiskTotalReadLatencyMinimum"`      //
	DiskTotalReadLatencyMinimumTime  time.Time `form:"DiskTotalReadLatencyMinimumTime"`  //
	DiskTotalWriteLatencyMaximum     float64   `form:"DiskTotalWriteLatencyMaximum"`     //
	DiskTotalWriteLatencyMaximumTime time.Time `form:"DiskTotalWriteLatencyMaximumTime"` //
	DiskTotalWriteLatencyAverage     float64   `form:"DiskTotalWriteLatencyAverage"`     //
	DiskTotalWriteLatencyMinimum     float64   `form:"DiskTotalWriteLatencyMinimum"`     //
	DiskTotalWriteLatencyMinimumTime time.Time `form:"DiskTotalWriteLatencyMinimumTime"` //
	DiskWriteMaximum                 float64   `form:"DiskWriteMaximum"`                 //
	DiskWriteMaximumTime             time.Time `form:"DiskWriteMaximumTime"`             //
	DiskWriteAverage                 float64   `form:"DiskWriteAverage"`                 //
	DiskWriteMinimum                 float64   `form:"DiskWriteMinimum"`                 //
	DiskWriteMinimumTime             time.Time `form:"DiskWriteMinimumTime"`             //
	DiskReadMaximum                  float64   `form:"DiskReadMaximum"`                  //
	DiskReadMaximumTime              time.Time `form:"DiskReadMaximumTime"`              //
	DiskReadAverage                  float64   `form:"DiskReadAverage"`                  //
	DiskReadMinimum                  float64   `form:"DiskReadMinimum"`                  //
	DiskReadMinimumTime              time.Time `form:"DiskReadMinimumTime"`              //
	NetUsageMaximum                  float64   `form:"NetUsageMaximum"`                  //
	NetUsageMaximumTime              time.Time `form:"NetUsageMaximumTime"`              //
	NetUsageAverage                  float64   `form:"NetUsageAverage"`                  //
	NetUsageMinimum                  float64   `form:"NetUsageMinimum"`                  //
	NetUsageMinimumTime              time.Time `form:"NetUsageMinimumTime"`              //
	CPUBilling                       float64   `form:"CPUBilling"`                       // в мегагерц часах
	MEMBilling                       float64   `form:"MEMBilling"`                       // в мегабайт часах
	DiskBilling                      float64   `form:"DiskBilling"`                      // по занимаемому месту в гигабайт часах
	NetBilling                       float64   `form:"NetBilling"`                       // в мегабайт часах
	Perf                             []HSPerf  `form:"Perf"`                             //
	PerfCount                        int64     `form:"PerfCount"`                        //
}
