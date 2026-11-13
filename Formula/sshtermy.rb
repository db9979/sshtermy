# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.388"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.388/sshtermy_darwin_arm64"
      sha256 "2f751e5e1dbcf8415d2b6d37b9e58b90ac30433e4d0439f0f5dfab5d6e059060"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.388/sshtermy_darwin_amd64"
      sha256 "16422fcbf3c069cd0da632a99a6402c182656e23c9ea3c25c401c72d3a2c35cb"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.388/sshtermy_linux_arm64"
      sha256 "c80ba95f80217a9b2625ea3f4a1467e01fc29d62c3c8e2aad31010aadbdb7f77"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.388/sshtermy_linux_amd64"
      sha256 "99155bc4ac1045420f1acc8e9ce6afbfa478ab084394cbe11cf2e3ac363baa29"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
