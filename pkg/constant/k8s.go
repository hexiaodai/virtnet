package constant

const (
	IPAMAnnotationPre = "ipam.virtnet.io"

	IPAMAnnoPodIPPools = IPAMAnnotationPre + "/ippools"
	IPAMAnnoPodIPPool  = IPAMAnnotationPre + "/ippool"

	IPAMAnnoPodSubnet = IPAMAnnotationPre + "/subnet"
)

const (
	IPPoolAnnotationPre = "ippool.virtnet.io"

	// IPPoolAnnoOwner     = IPPoolAnnotationPre + "/owner"
	// IPPoolAnnoOwned     = IPPoolAnnotationPre + "/owned"
	// IPPoolAnnoCount     = IPPoolAnnotationPre + "/count"
	// IPPoolAnnoSharedOut = IPPoolAnnotationPre + "/shared-out"
	// IPPoolAnnoSharedIn  = IPPoolAnnotationPre + "/shared-in"
)

const (
	IPPoolLabelPre       = "ippool.virtnet.io"
	IPPoolLabelOwner     = IPPoolLabelPre + "/owner"
	IPPoolLabelOwned     = IPPoolLabelPre + "/owned"
	IPPoolLabelExclusive = IPPoolLabelPre + "/exclusive"
	// IPPoolLabelCount = IPPoolAnnotationPre + "/count"
)

const (
	SubnetLabelPre   = "subnet.virtnet.io"
	SubnetLabelOwner = IPPoolLabelPre + "/owner"
	// SubnetLabelName = SubnetLabelPre + "/name"

	// SubnetLabelOwned = SubnetLabelPre + "/owned"
	// IPPoolLabelCount = IPPoolAnnotationPre + "/count"
)

const (
// BarrelLabelPre   = "barrel.virtnet.io"
// BarrelLabelOwner = BarrelLabelPre + "/owner"
)
