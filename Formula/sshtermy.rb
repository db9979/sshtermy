# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.154"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.154/sshtermy_darwin_arm64"
      sha256 "3724dfd2a2fe0fbf962144db762a60eb4db2c46c1d08f0434f0f9534a062c017"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.154/sshtermy_darwin_amd64"
      sha256 "56710e4707ec1684f9295cebbfc2175eb2e0de5ba4773fca26f122c416f35e08"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.154/sshtermy_linux_arm64"
      sha256 "927fdad805e5674346aa906711ecc9b02b13f7eeab04d37081a523ace5e35079"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.154/sshtermy_linux_amd64"
      sha256 "7ae2fb089b1bbeeb0e4a7aa4e8402f6354ed21ec0893e972348821c0a98b87f4"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
