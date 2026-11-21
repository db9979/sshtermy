# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.436"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.436/sshtermy_darwin_arm64"
      sha256 "d0a71aeedc46d671aac5811eab6ac3dad2d7a5c3a5cd8f790a7223335605f0f7"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.436/sshtermy_darwin_amd64"
      sha256 "211d94f2f436d14ce5f5eee992237973d26eafef305d8f57e152be19a2c57e72"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.436/sshtermy_linux_arm64"
      sha256 "832aeb514f85dce4459170da971568a71dfff9874cf3750f2ee2e406e71b4b97"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.436/sshtermy_linux_amd64"
      sha256 "28e52456e3201e0e773dbfb11d8541e367dc02ee9b78d9d9dc90d51ad1dd7ad7"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
