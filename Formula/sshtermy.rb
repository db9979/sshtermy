# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.438"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.438/sshtermy_darwin_arm64"
      sha256 "086733171fb63758adc917b8a17ba560ddf9d00d78cb372a5e0c70fa9cdbd0c6"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.438/sshtermy_darwin_amd64"
      sha256 "9a42aed1e6c27a50087375ae6af441f91110dd92fc32bb4bd4ad1e49381f608a"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.438/sshtermy_linux_arm64"
      sha256 "0603b0227dadc4efd11d35a0df45dad7e887c837f3f0927d6293a0ec08c19540"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.438/sshtermy_linux_amd64"
      sha256 "de774f9fbc3db341e77df8130c099773c96658192bbf5558568a0907d9c7820b"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
