// Package buildinfo, derleme ve protokol sürümü gibi derleme zamanı
// bilgilerini tek bir yerden sunar.
package buildinfo

import "github.com/a2aproject/a2a-go/v2/a2a"

// Version, uygulamanın derleme sürümüdür. Release sürecinde -ldflags ile
// güncellenebilir.
var Version = "dev"

// A2AProtocolVersion, a2a-go SDK'nın uyguladığı A2A protokol sürümüdür.
const A2AProtocolVersion = a2a.Version
