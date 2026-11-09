# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.308"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.308/sshtermy_darwin_arm64"
      sha256 "cdc93190c3d5ef084f9bfe1380bb4bbaa00214fb7fb55c5d5db08dff515b78e3"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.308/sshtermy_darwin_amd64"
      sha256 "91ba2c37decbc8b42d5f2fae37987ceaf91a2f85ab229e84d2d3e6773e218520"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.308/sshtermy_linux_arm64"
      sha256 "6623e88eface7043c86c8c401259e6b23e6d240daba47077c5eee8ff450fbe2d"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.308/sshtermy_linux_amd64"
      sha256 "c94f68de1a896a279be6de113988781db832dbfe7ee9f5873d24ec62e0efad9b"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
