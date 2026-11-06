# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.183"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.183/sshtermy_darwin_arm64"
      sha256 "631366d5588d2eeaaa34d48eb05695fba729dae244965c70c8baaf15c7c7031d"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.183/sshtermy_darwin_amd64"
      sha256 "4fac36f49a3cbdbe35b63d4306ef393e79e93e0986660ccdaeecaab265a115bf"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.183/sshtermy_linux_arm64"
      sha256 "86e7e2a74c38ab4939392f599da76840e70c75031631591ad7937c484a1e4221"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.183/sshtermy_linux_amd64"
      sha256 "f4f7d5dda54d7a9324544e621865eb23f2f9f92a8eb39a10d6ec787862cc4019"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
