# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.217"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.217/sshtermy_darwin_arm64"
      sha256 "ee7d958a68bd389ecf25e982d2429fe52a685df81b20c6c38d5130745d69c3d4"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.217/sshtermy_darwin_amd64"
      sha256 "79778961cc8d427ccf8767b7c8116af44180a33fb5ca7bc88838d921bb386038"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.217/sshtermy_linux_arm64"
      sha256 "15f66d7582216249409abb753bfdf0be4889bc64b47c7f84d46b9c24f55838bb"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.217/sshtermy_linux_amd64"
      sha256 "e8668289714702bc31d71e5fa603f7e23be8c26a8e7d0c73e5661051069ba788"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
