# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.163"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.163/sshtermy_darwin_arm64"
      sha256 "ade67ba09470d2373fa29aa6aaf27ecc9ab83e09309b228f6fc670db70c9eafe"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.163/sshtermy_darwin_amd64"
      sha256 "7648792f3c816b7694ed7e9e1f62d6bc1d4815a3d2757649d75441a9fb83b54f"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.163/sshtermy_linux_arm64"
      sha256 "67fb5c5f388ddaf5ae72bd4b85cde6c3835db43163acefd5c29735e1f71c3715"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.163/sshtermy_linux_amd64"
      sha256 "38b40214f62434d811f5257d0cb8d82b88900026508fd6bbc04a6a0ed854002f"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
