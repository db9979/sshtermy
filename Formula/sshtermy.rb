# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.332"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.332/sshtermy_darwin_arm64"
      sha256 "0fbd947af80c9df7c6f437b0b1458b51fd72440117c5f976047533be54b79952"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.332/sshtermy_darwin_amd64"
      sha256 "6930a296417f8a439581bcb51934fcc1adeeb565f5de478c3cb8fa9f7d49d9e4"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.332/sshtermy_linux_arm64"
      sha256 "b5a40f49428954917711b72c9e19026b26d08c33704f798f9c43a0bea9cacda6"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.332/sshtermy_linux_amd64"
      sha256 "d1442a885596d594cd55947f35e3e275fc23b2e87b41fa7e9aa188d71a2f6ef3"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
