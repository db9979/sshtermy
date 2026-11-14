# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.403"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.403/sshtermy_darwin_arm64"
      sha256 "210cdd0255df377f51939f38f1a6a35b553a5f5850767a375066f6b4b18de123"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.403/sshtermy_darwin_amd64"
      sha256 "2208bce648099331d58c955012b77a7e03f87f113850a4b2b15c8efaec5d1c54"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.403/sshtermy_linux_arm64"
      sha256 "62354eeebecb12c6987ad30feb7d85a0046d314a943e235657614df7e7c44f79"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.403/sshtermy_linux_amd64"
      sha256 "a55a99aa71a315428f11179c8263f51e4e7d3c5d3bec9fbe4c14c47705b43acd"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
