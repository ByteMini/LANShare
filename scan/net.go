package scan

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"log"
	"strings"
	"time"
)

// 发现当前主机中所有的网络设备
func FindDevices() {
	// 得到所有的网络设备
	devices, err := pcap.FindAllDevs()
	if err != nil {
		log.Fatal(err)
	}

	// 打印设备信息
	fmt.Println("All Devices:")
	for _, device := range devices {
		fmt.Println("\nName: ", device.Name)
		fmt.Println("Description: ", device.Description)
		fmt.Println("Devices addresses:")
		for _, address := range device.Addresses {
			fmt.Println("- IP address: ", address.IP)
			fmt.Println("- Subnet mask: ", address.Netmask)
		}
	}
}

// 解析获取到的数据包
func ProcessPacket(packet gopacket.Packet) {
	// 遍历包中所有OSI模型的各层
	allLayer := packet.Layers()
	for _, layer := range allLayer {
		fmt.Printf("layer: %v\n", layer.LayerType())
	}
	fmt.Println(strings.Repeat("-", 50))

	// 解析链路层数据
	ethernetLayer := packet.Layer(layers.LayerTypeEthernet)
	if ethernetLayer != nil {
		ethernetPacket, _ := ethernetLayer.(*layers.Ethernet)
		fmt.Printf("Ethernet ==> type: %v, source mac: %v, destination mac: %v\n",
			ethernetPacket.EthernetType, ethernetPacket.SrcMAC, ethernetPacket.DstMAC)
	}

	// 解析网络层数据
	ipLayer := packet.Layer(layers.LayerTypeIPv4)
	if ipLayer != nil {
		ip, _ := ipLayer.(*layers.IPv4)
		fmt.Printf("IP ==> protocal: %v, from: %v, to: %v\n", ip.Protocol, ip.SrcIP, ip.DstIP)
	}

	// 解析运输层数据
	tcpLayer := packet.Layer(layers.LayerTypeTCP)
	if tcpLayer != nil {
		tcp, _ := tcpLayer.(*layers.TCP)
		fmt.Printf("TCP ==> source_port: %v, destination_port: %v\n", tcp.SrcPort, tcp.DstPort)
	}

	udpLayer := packet.Layer(layers.LayerTypeUDP)
	if udpLayer != nil {
		udp, _ := udpLayer.(*layers.UDP)
		fmt.Printf("UDP ==> source_port: %v, destination_port: %v\n", udp.SrcPort, udp.DstPort)
	}

	// 解析应用层数据
	appLayer := packet.ApplicationLayer()
	if appLayer != nil {
		fmt.Printf("application ==> payload: %v\n", string(appLayer.Payload()))
	}

	// 处理错误
	err := packet.ErrorLayer()
	if err != nil {
		fmt.Printf("decode packet err: %v\n", err)
	}

	fmt.Println(strings.Repeat("-", 50))
}

// 使用指定设备获取数据包
func GetPacket(device string) {
	var (
		snapshotLength int32 = 1024
		promiscuous          = false
		timeout              = 30 * time.Second
		handle         *pcap.Handle
		err            error
	)

	handle, err = pcap.OpenLive(device, snapshotLength, promiscuous, timeout)
	if err != nil {
		log.Fatal(err)
	}

	defer handle.Close()

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	for packet := range packetSource.Packets() {
		ProcessPacket(packet)
		//fmt.Println(packet.Dump())
	}
}
