# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.310"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.310/sshtermy_darwin_arm64"
      sha256 "69a907631c203d5309655a6a48193e5f62cf35acd7053713600b2019f922295e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.310/sshtermy_darwin_amd64"
      sha256 "2058fba4666ce9570a0ad1695a2cbc7ced91c5abd546a1c89c7e4ba9cc256653"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.310/sshtermy_linux_arm64"
      sha256 "231bd1b327ea50701e5937a9732c5961d9c97e4c8a1402e563ddbe1d538f1e90"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.310/sshtermy_linux_amd64"
      sha256 "ee1ec5d050034f44b0d0f39c74644b459ac7906ad2ce14f4d1f7f56551395d17"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
