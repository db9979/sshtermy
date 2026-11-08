# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.263"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.263/sshtermy_darwin_arm64"
      sha256 "ebc138ac86f0ad4077f0342b4a33dc2341fdb436462bdeb04e78fcca6f52762a"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.263/sshtermy_darwin_amd64"
      sha256 "8c1a8ee7d38f1db7453d915c95c6b25eeffea2a6ad3406b44a1cb3e40af86cf2"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.263/sshtermy_linux_arm64"
      sha256 "bf6b29d2300a9cdf92259f70311dbb6b9bc3a846670dd8420d480ae24efa2f92"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.263/sshtermy_linux_amd64"
      sha256 "25d3469d076e67d7f939e0d39ab972e5a8d348cb29ffb14c59f90a6437299e70"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
