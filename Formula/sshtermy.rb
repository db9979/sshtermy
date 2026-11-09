# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.300"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.300/sshtermy_darwin_arm64"
      sha256 "43c790b28caf2a109eede8a3017c1773a7d265d45bee8fd0ded2cc1a91bcc303"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.300/sshtermy_darwin_amd64"
      sha256 "26eb245f35ac0c0a2c2d0fe94be02e9a2dc0a3b15f9c6cdb764c2c43d3bc9359"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.300/sshtermy_linux_arm64"
      sha256 "fdab442c23c96103d085813864e66448cc3c0b2757b14db21336140e050d829e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.300/sshtermy_linux_amd64"
      sha256 "75867ef2bce20cc24acc9d05913fb81830142febf8bed64e349de96ff48727e5"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
