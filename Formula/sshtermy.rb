# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.340"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.340/sshtermy_darwin_arm64"
      sha256 "934614de581bdbba1d17649c04d809394789a16f51ad3c0ae02800d7ad125591"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.340/sshtermy_darwin_amd64"
      sha256 "f5bfaf866ff5c24e4a184b1d4a4d1601af8ac7fd8821491c609c3425e19c7b0e"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.340/sshtermy_linux_arm64"
      sha256 "4c69393106f602ada1f3a15f7cf595d105bfa85fa975886a23b8ddfc7813e6f3"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.340/sshtermy_linux_amd64"
      sha256 "6400b8ef58f0d56284e2443c7c800fe77f2eddf883936c085f8404f9691cb2bc"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
