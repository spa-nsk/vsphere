package main

import (
	"time"

	"github.com/spa-nsk/vsphere"
)

func main() {
	infra := vsphere.GlobalInitVMInfrastructure()                       //инициализируем инвраструктуру и получаем указатель на нее
	username := "login-name@domain"                                     //имя учетной записи для авторизации на управляющем vcenter
	password := "password"                                              //пароль для учетной записи
	infra.AddVCenter("vc-01.domain", username, password)                //регистрируем управляющий центр vc-01.domain
	infra.AddVCenter("vc-02.domain", username, password)                //регистрируем управляющий центр vc-02.domain
	infra.AddVCenter("vc-03.domain", username, password)                //регистрируем управляющий центр vc-03.domain
	infra.AddVCenter("vc-04.domain", "demo@new-domain", "new-password") //регистрируем управляющий центр vc-04.domain
	infra.LoadDCHSCLInfo()                                              //загружаем данные по хостам и кластерам
	infra.LoadVMInfo()                                                  //загружаем данные по виртуальным машинам
	for ; ; time.Sleep(20 * time.Second) {                              //в бесконечном цикле опрашиваем метрики и выводим пример на экран
		infra.LoadHostAndVMMetric() // опрашиваем каждые 20 секунд
		infra.ListVMPerf()          //выводим список метрик
	}
}
