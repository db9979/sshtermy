# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.254"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.254/sshtermy_darwin_arm64"
      sha256 "9f4a97f6ff09abfa2cb258ce579a5bc5f2b01f075fb71d030dd9a4cdd74c9db2"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.254/sshtermy_darwin_amd64"
      sha256 "aa0eb2b5cc5e1f3c37ae46fe7f9dc64e5c1b78ed3905654acaee7390baa35007"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.254/sshtermy_linux_arm64"
      sha256 "77daa28584bba2cd5f8683880cd41be2d23c8b9868d300ffb844948a4971c6d0"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.254/sshtermy_linux_amd64"
      sha256 "733cfd4389a1d307277b5ac000f92628ac1fe75483eda1da58bf658d7ae2097d"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
